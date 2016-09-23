package vspherejanitor

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/pkg/errors"
	"github.com/rcrowley/go-metrics"
	"github.com/travis-ci/vsphere-janitor/log"
	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/types"
)

var (
	errVMFolderNotFound = fmt.Errorf("VM folder not found")
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

func (j *Janitor) Cleanup(ctx context.Context, path string) error {
	sem := make(chan struct{}, j.opts.Concurrency)
	wg := sync.WaitGroup{}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
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
		return errVMFolderNotFound
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
			log.WithContext(ctx).WithField("ref", vmRef).Infof("skipping non-vm type %T", vmRef)
			continue
		}

		atomic.AddInt64(&totalVMs, int64(1))

		err := j.handleVM(ctx, vm, &wg, sem)
		if err != nil {
			vmErrors = append(vmErrors, err)
			log.WithContext(ctx).WithError(err).Error("error handling VM")
		}
	}

	wg.Wait()

	metrics.GetOrRegisterGauge("vsphere.janitor.cleanup.vms.total", metrics.DefaultRegistry).Update(totalVMs)
	return nil
}

func (j *Janitor) handleVM(ctx context.Context,
	vm *object.VirtualMachine, wg *sync.WaitGroup, sem chan (struct{})) (err error) {

	mvm := &mo.VirtualMachine{}

	err = vm.Properties(ctx, vm.Reference(), []string{"config", "summary"}, mvm)
	if err != nil {
		return err
	}

	vmName := "<unnamed>"
	if mvm.Config != nil {
		vmName = mvm.Config.Name
	}

	logger := log.WithContext(ctx).WithField("vm", vmName)

	defer func() {
		panicErr := recover()
		if panicErr != nil {
			err = panicErr.(error)
		}
	}()

	uptimeSecs := mvm.Summary.QuickStats.UptimeSeconds

	if j.opts.SkipZeroUptime && uptimeSecs == 0 && mvm.Summary.Runtime.BootTime == nil {
		logger.Info("instance has 0 uptime, skipping")
		return nil
	}

	bootTime := mvm.Summary.Runtime.BootTime

	if j.opts.SkipNoBootTime && bootTime == nil {
		logger.Info("instance has no boot time, skipping")
		return nil
	}

	if bootTime != nil {
		bootedAgo := time.Now().UTC().Sub(*mvm.Summary.Runtime.BootTime)
		logger.WithField("booted_ago", bootedAgo).Info("instance booted ago")
	}

	uptime := time.Duration(uptimeSecs) * time.Second
	if j.opts.SkipZeroUptime && uptime < j.opts.Cutoff &&
		mvm.Summary.Runtime.PowerState != types.VirtualMachinePowerStatePoweredOff {

		logger.WithField("uptime", uptime).WithField("power_state", mvm.Summary.Runtime.PowerState).Info("skipping instance")
		return nil
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		err := j.powerOffAndDestroy(ctx, logger, sem, uptime, mvm, vm)
		if err != nil {
			logger.WithError(err).Error("error powering off and destroying instance")
		}
	}()
	return nil
}

func (j *Janitor) powerOffAndDestroy(ctx context.Context, logger logrus.FieldLogger, sem chan (struct{}), uptime time.Duration, mvm *mo.VirtualMachine, vm *object.VirtualMachine) (err error) {
	sem <- struct{}{}
	defer func() {
		panicErr := recover()
		if panicErr != nil {
			err = panicErr.(error)
		}
		<-sem
	}()

	logger.WithField("uptime", uptime).Info("handling poweroff and destroy of instance")

	if mvm.Summary.Runtime.PowerState == types.VirtualMachinePowerStatePoweredOn {
		logger.Info("powering off instance")

		task, err := vm.PowerOff(ctx)
		if err != nil {
			return errors.Wrap(err, "couldn't create power off task")
		}

		err = task.Wait(ctx)
		if err != nil {
			return errors.Wrap(err, "couldn't power off instance")
		}

		metrics.GetOrRegisterMeter("vsphere.janitor.cleanup.vms.poweroff", metrics.DefaultRegistry).Mark(1)
	}

	if j.opts.SkipDestroy {
		logger.Info("skipping destroy step")
		return nil
	}

	logger.Info("destroying instance")

	task, err := vm.Destroy(ctx)
	if err != nil {
		return errors.Wrap(err, "couldn't create destroy task")
	}

	err = task.Wait(ctx)
	if err != nil {
		return errors.Wrap(err, "couldn't destroy instance")
	}

	logger.Info("destroyed instance")
	metrics.GetOrRegisterMeter("vsphere.janitor.cleanup.vms.destroy", metrics.DefaultRegistry).Mark(1)

	return nil
}
