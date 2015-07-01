package vspherejanitor

import (
	"fmt"
	"log"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rcrowley/go-metrics"
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
			Cutoff:         2 * time.Hour,
			Concurrency:    1,
			RatePerSecond:  5,
			SkipZeroUptime: true,
			SkipNoBootTime: true,
		}
	}

	return &Janitor{
		u:    u,
		opts: opts,
	}
}

type JanitorOpts struct {
	Cutoff         time.Duration
	SkipDestroy    bool
	Concurrency    int
	RatePerSecond  int
	SkipZeroUptime bool
	SkipNoBootTime bool
}

func (j *Janitor) Cleanup(path string) error {
	sem := make(chan struct{}, j.opts.Concurrency)
	wg := sync.WaitGroup{}
	ctx := context.Background()
	throttle := time.Tick(time.Second / time.Duration(j.opts.RatePerSecond))

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
	totalVMs := int64(0)

	for _, vmRef := range vms {
		<-throttle

		vm, ok := vmRef.(*object.VirtualMachine)
		if !ok {
			log.Printf("Skipping non-vm %T: %v", vmRef, vmRef)
			continue
		}

		totalVMs = atomic.AddInt64(&totalVMs, int64(1))

		err := j.handleVM(vm, ctx, wg, sem)
		if err != nil {
			vmErrors = append(vmErrors, err)
			log.Printf("Error handling vm: %v", err)
		}
	}

	wg.Wait()

	metrics.GetOrRegisterGauge("vsphere.janitor.cleanup.vms.total", metrics.DefaultRegistry).Update(totalVMs)
	return nil
}

func (j *Janitor) handleVM(vm *object.VirtualMachine,
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

	uptimeSecs := mvm.Summary.QuickStats.UptimeSeconds

	if j.opts.SkipZeroUptime && uptimeSecs == 0 && mvm.Summary.Runtime.BootTime == nil {
		log.Printf("instance has 0 uptime, skipping %v", vmName)
		return nil
	}

	bootTime := mvm.Summary.Runtime.BootTime

	if j.opts.SkipNoBootTime && bootTime == nil {
		log.Printf("instance has no boot time, skipping %v", vmName)
		return nil
	}

	if bootTime != nil {
		bootedAgo := time.Now().UTC().Sub(*mvm.Summary.Runtime.BootTime)
		log.Printf("instance booted_ago=%v %v", bootedAgo, vmName)
	}

	uptime := time.Duration(0) * time.Second

	uptime = time.Duration(uptimeSecs) * time.Second
	if j.opts.SkipZeroUptime && uptime < j.opts.Cutoff &&
		mvm.Summary.Runtime.PowerState != types.VirtualMachinePowerStatePoweredOff {

		log.Printf("skipping instance %v uptime=%v power_state=%v",
			vmName, uptime, mvm.Summary.Runtime.PowerState)
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

			metrics.GetOrRegisterMeter("vsphere.janitor.cleanup.vms.poweroff", metrics.DefaultRegistry).Mark(1)
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
		metrics.GetOrRegisterMeter("vsphere.janitor.cleanup.vms.destroy", metrics.DefaultRegistry).Mark(1)
	}()
	return nil
}
