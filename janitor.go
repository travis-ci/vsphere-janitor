package vspherejanitor

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/pkg/errors"
	"github.com/rcrowley/go-metrics"
	"github.com/travis-ci/vsphere-janitor/log"
)

type Janitor struct {
	vmLister VMLister
	opts     *JanitorOpts
}

func NewJanitor(vmLister VMLister, opts *JanitorOpts) *Janitor {
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
		vmLister: vmLister,
		opts:     opts,
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
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sem := make(chan struct{}, j.opts.Concurrency)
	wg := sync.WaitGroup{}
	throttle := time.Tick(time.Second / time.Duration(j.opts.RatePerSecond))

	vms, err := j.vmLister.ListVMs(ctx, path)
	if err != nil {
		return errors.Wrap(err, "couldn't list VMs")
	}

	vmErrors := []error{}
	totalVMs := int64(0)

	for _, vm := range vms {
		<-throttle

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
	vm VirtualMachine, wg *sync.WaitGroup, sem chan (struct{})) (err error) {
	logger := log.WithContext(ctx).WithField("vm", vm.Name())

	defer func() {
		panicErr := recover()
		if panicErr != nil {
			err = panicErr.(error)
		}
	}()

	uptimeSecs := int(vm.Uptime().Seconds())

	if j.opts.SkipZeroUptime && uptimeSecs == 0 && vm.BootTime() == nil {
		logger.Info("instance has 0 uptime, skipping")
		return nil
	}

	bootTime := vm.BootTime()

	if j.opts.SkipNoBootTime && bootTime == nil {
		logger.Info("instance has no boot time, skipping")
		return nil
	}

	if bootTime != nil {
		bootedAgo := time.Now().UTC().Sub(*bootTime)
		logger.WithField("booted_ago", bootedAgo).Info("instance booted at")
	}

	uptime := time.Duration(uptimeSecs) * time.Second
	if j.opts.SkipZeroUptime && uptime < j.opts.Cutoff && vm.PoweredOn() {
		logger.WithField("uptime", uptime).WithField("powered_on", vm.PoweredOn()).Info("skipping instance")
		return nil
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		err := j.powerOffAndDestroy(ctx, logger, sem, vm)
		if err != nil {
			logger.WithError(err).Error("error powering off and destroying instance")
		}
	}()
	return nil
}

func (j *Janitor) powerOffAndDestroy(ctx context.Context, logger logrus.FieldLogger, sem chan (struct{}), vm VirtualMachine) (err error) {
	sem <- struct{}{}
	defer func() {
		panicErr := recover()
		if panicErr != nil {
			err = panicErr.(error)
		}
		<-sem
	}()

	logger.WithField("uptime", vm.Uptime()).Info("handling poweroff and destroy of instance")

	if vm.PoweredOn() {
		logger.Info("powering off instance")

		err := vm.PowerOff(ctx)
		if err != nil {
			return errors.Wrap(err, "error powering off VM")
		}

		metrics.GetOrRegisterMeter("vsphere.janitor.cleanup.vms.poweroff", metrics.DefaultRegistry).Mark(1)
	}

	if j.opts.SkipDestroy {
		logger.Info("skipping destroy step")
		return nil
	}

	logger.Info("destroying instance")

	err = vm.Destroy(ctx)
	if err != nil {
		return errors.Wrap(err, "error destroying VM")
	}

	logger.Info("destroyed instance")
	metrics.GetOrRegisterMeter("vsphere.janitor.cleanup.vms.destroy", metrics.DefaultRegistry).Mark(1)

	return nil
}
