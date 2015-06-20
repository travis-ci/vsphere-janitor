package vspherejanitor

import (
	"fmt"
	"log"
	"net/url"
	"sync"
	"time"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	"golang.org/x/net/context"
)

var (
	vmFolderNotFoundError = fmt.Errorf("VM folder not found")
)

type Janitor struct {
	u    *url.URL
	opts *JanitorOpts
}

func NewJanitor(u *url.URL, opts *JanitorOpts) *Janitor {
	if opts == nil {
		opts = &JanitorOpts{
			Cutoff:      2 * time.Hour,
			Concurrency: 1,
		}
	}

	return &Janitor{
		u:    u,
		opts: opts,
	}
}

type JanitorOpts struct {
	Cutoff      time.Duration
	SkipDestroy bool
	Concurrency int
}

type JanitorStats struct {
	NPoweredOff int
	NDestroyed  int
}

func (j *Janitor) Cleanup(path string) error {
	sem := make(chan struct{}, j.opts.Concurrency)
	wg := sync.WaitGroup{}
	stats := &JanitorStats{}
	ctx := context.Background()

	client, err := govmomi.NewClient(ctx, j.u, true)
	if err != nil {
		return err
	}

	searchIndex := object.NewSearchIndex(client.Client)

	folderRef, err := searchIndex.FindByInventoryPath(ctx, path)
	if err != nil {
		return err
	}

	if folderRef == nil {
		return vmFolderNotFoundError
	}

	folder, ok := folderRef.(*object.Folder)
	if !ok {
		return fmt.Errorf("VM folder is not a folder but a %T", folderRef)
	}

	vms, err := folder.Children(ctx)
	if err != nil {
		return err
	}

	vmErrors := []error{}

	for _, vmRef := range vms {
		vm, ok := vmRef.(*object.VirtualMachine)
		if !ok {
			log.Printf("Skipping non-vm %T: %v", vmRef, vmRef)
			continue
		}

		err := j.handleVM(vm, stats, ctx, wg, sem)
		if err != nil {
			vmErrors = append(vmErrors, err)
			log.Printf("Error handling vm: %v", err)
		}
	}

	wg.Wait()
	log.Printf("finished powered_off=%v destroyed=%v", stats.NPoweredOff, stats.NDestroyed)
	return nil
}

func (j *Janitor) handleVM(vm *object.VirtualMachine, stats *JanitorStats,
	ctx context.Context, wg sync.WaitGroup, sem chan (struct{})) (panicErr error) {

	mvm := &mo.VirtualMachine{}

	err := vm.Properties(ctx, vm.Reference(), []string{"config", "summary"}, mvm)
	if err != nil {
		return err
	}

	vmName := "<unnamed>"
	if mvm.Config != nil {
		vmName = mvm.Config.Name
	}

	panicErrAddr := &panicErr
	defer func() {
		err := recover()
		if err != nil {
			(*panicErrAddr) = err.(error)
			log.Printf("ERROR: recovered from err for %v: %v", vmName, err)
		}
	}()

	if mvm.Summary.QuickStats.UptimeSeconds == 0 && mvm.Summary.Runtime.BootTime == nil {
		log.Printf("instance has 0 uptime, skipping %v", vmName)
		return nil
	}

	if mvm.Summary.Runtime.BootTime == nil {
		log.Printf("instance has no boot time, skipping %v", vmName)
		return nil
	}

	bootedAgo := time.Now().UTC().Sub(*mvm.Summary.Runtime.BootTime)
	log.Printf("%v booted_ago=%v", vmName, bootedAgo)

	uptime := time.Duration(mvm.Summary.QuickStats.UptimeSeconds) * time.Second

	if uptime < j.opts.Cutoff && mvm.Summary.Runtime.PowerState != types.VirtualMachinePowerStatePoweredOff {
		log.Printf("skipping instance %v uptime=%v power_state=%v", vmName, uptime, mvm.Summary.Runtime.PowerState)
		return nil
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		sem <- struct{}{}
		defer func() {
			err := recover()
			if err != nil {
				log.Printf("ERROR: recovered from err for %v: %v", vmName, err)
			}
			<-sem
		}()

		log.Printf("handling poweroff and destroy of instance %v uptime=%v", vmName, uptime)

		if mvm.Summary.Runtime.PowerState == types.VirtualMachinePowerStatePoweredOn {
			log.Printf("powering off instance %v", vmName)

			task, err := vm.PowerOff(ctx)
			if err != nil {
				log.Printf("couldn't power off instance %s: %s", vmName, err)
				return
			}

			err = task.Wait(ctx)
			if err != nil {
				log.Printf("couldn't power off instance %s: %s", vmName, err)
				return
			}

			stats.NPoweredOff++
		}

		if j.opts.SkipDestroy {
			log.Printf("skipping destroy step for %s", vmName)
			return
		}

		log.Printf("destroying instance %v", vmName)

		task, err := vm.Destroy(ctx)
		if err != nil {
			log.Printf("couldn't destroy instance %v: %s", vmName, err)
			return
		}

		err = task.Wait(ctx)
		if err != nil {
			log.Printf("couldn't destroy instance %v: %s", vmName, err)
			return
		}

		log.Printf("destroyed instance %s", vmName)
		stats.NDestroyed++
	}()
	return nil
}