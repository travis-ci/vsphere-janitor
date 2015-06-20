package vmwarejanitor

import (
	"log"
	"net/url"
	"sync"
	"time"

	"github.com/codegangsta/cli"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
	"golang.org/x/net/context"
)

func RunCleanup(c *cli.Context) {
	u, err := url.Parse(c.String("vsphere-url"))
	if err != nil {
		log.Fatal(err)
	}

	vmPath := c.String("vsphere-vm-path")
	if vmPath == "" {
		log.Fatal("missing vsphere vm path")
	}

	skipDestroy := c.Bool("skip-destroy")
	cutoffDuration := c.Duration("cutoff")
	concurrency := c.Int("concurrency")

	sem := make(chan struct{}, concurrency)

	wg := sync.WaitGroup{}

	nPoweredOff := 0
	nDestroyed := 0

	ctx := context.Background()
	client, err := govmomi.NewClient(ctx, u, true)
	if err != nil {
		log.Fatal(err)
	}

	searchIndex := object.NewSearchIndex(client.Client)

	folderRef, err := searchIndex.FindByInventoryPath(ctx, vmPath)
	if err != nil {
		log.Fatal(err)
	}

	if folderRef == nil {
		log.Fatal("VM folder not found")
	}

	folder, ok := folderRef.(*object.Folder)
	if !ok {
		log.Fatalf("VM folder is not a folder but a %T", folderRef)
	}

	vms, err := folder.Children(ctx)
	if err != nil {
		log.Fatal(err)
	}

	for _, vmRef := range vms {
		func() {
			vm, ok := vmRef.(*object.VirtualMachine)
			if !ok {
				return
			}

			var mvm mo.VirtualMachine
			err := vm.Properties(ctx, vm.Reference(), []string{"config", "summary"}, &mvm)
			if err != nil {
				log.Printf("couldn't get instance properties: %s", err)
				return
			}

			vmName := "<unnamed>"
			if mvm.Config != nil {
				vmName = mvm.Config.Name
			}

			defer func() {
				err := recover()
				if err != nil {
					log.Printf("ERROR: recovered from err for %v: %v", vmName, err)
				}
			}()

			if mvm.Summary.QuickStats.UptimeSeconds == 0 && mvm.Summary.Runtime.BootTime == nil {
				log.Printf("instance has 0 uptime, skipping %v", vmName)
				return
			}

			if mvm.Summary.Runtime.BootTime == nil {
				log.Printf("instance has no boot time, skipping %v", vmName)
				return
			}

			bootedAgo := time.Now().UTC().Sub(*mvm.Summary.Runtime.BootTime)
			log.Printf("%v booted_ago=%v", vmName, bootedAgo)

			uptime := time.Duration(mvm.Summary.QuickStats.UptimeSeconds) * time.Second

			if uptime < cutoffDuration && mvm.Summary.Runtime.PowerState != types.VirtualMachinePowerStatePoweredOff {
				log.Printf("skipping instance %v uptime=%v power_state=%v", vmName, uptime, mvm.Summary.Runtime.PowerState)
				return
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

					nPoweredOff++
				}

				if skipDestroy {
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
				nDestroyed++
			}()
		}()
	}

	wg.Wait()
	log.Printf("finished powered_off=%v destroyed=%v", nPoweredOff, nDestroyed)
}
