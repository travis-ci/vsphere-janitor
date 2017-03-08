package vspherejanitor

import (
	"context"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/pkg/errors"
	"github.com/rcrowley/go-metrics"
	"github.com/travis-ci/vsphere-janitor/log"
)

type Janitor struct {
	vmLister VMLister
	opts     *JanitorOpts

	zeroUptimeFirstSeenMutex sync.Mutex
	zeroUptimeFirstSeen      map[string]time.Time
}

func NewJanitor(vmLister VMLister, opts *JanitorOpts) *Janitor {
	if opts == nil {
		opts = &JanitorOpts{
			Cutoff:         2 * time.Hour,
			Concurrency:    1,
			RatePerSecond:  5,
			SkipNoBootTime: true,
		}
	}

	return &Janitor{
		vmLister:            vmLister,
		opts:                opts,
		zeroUptimeFirstSeen: make(map[string]time.Time),
	}
}

type JanitorOpts struct {
	Cutoff           time.Duration
	ZeroUptimeCutoff time.Duration
	SkipDestroy      bool
	Concurrency      int
	RatePerSecond    int
	SkipNoBootTime   bool
}

func (j *Janitor) Cleanup(ctx context.Context, path string, now time.Time) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sem := make(chan struct{}, j.opts.Concurrency)
	wg := sync.WaitGroup{}
	throttle := time.Tick(time.Second / time.Duration(j.opts.RatePerSecond))

	vms, err := j.vmLister.ListVMs(ctx, path)
	if err != nil {
		return errors.Wrap(err, "couldn't list VMs")
	}

	for _, vm := range vms {
		<-throttle

		err := j.handleVM(ctx, vm, &wg, sem, now)
		if err != nil {
			log.WithContext(ctx).WithError(err).Error("error handling VM")
		}
	}

	j.cleanupFirstSeen(vms)

	wg.Wait()

	metrics.GetOrRegisterGauge("vsphere.janitor.cleanup.vms.total", metrics.DefaultRegistry).Update(int64(len(vms)))
	return nil
}

func (j *Janitor) cleanupFirstSeen(vms []VirtualMachine) {
	j.zeroUptimeFirstSeenMutex.Lock()
	defer j.zeroUptimeFirstSeenMutex.Unlock()

	vmExists := make(map[string]bool, len(vms))
	for _, vm := range vms {
		vmExists[vm.ID()] = true
	}

	for id := range j.zeroUptimeFirstSeen {
		if !vmExists[id] {
			delete(j.zeroUptimeFirstSeen, id)
		}
	}
}

func (j *Janitor) handleVM(ctx context.Context,
	vm VirtualMachine, wg *sync.WaitGroup, sem chan (struct{}), now time.Time) (err error) {
	logger := log.WithContext(ctx).WithField("vm", vm.Name())

	defer func() {
		panicErr := recover()
		if panicErr != nil {
			err = panicErr.(error)
		}
	}()

	uptimeSecs := int(vm.Uptime().Seconds())

	if uptimeSecs == 0 && vm.BootTime() == nil {
		if vm.ID() == "" {
			logger.Info("VM doesn't have ID yet, skipping")
			return nil
		}

		firstSeen, ok := j.getZeroUptimeFirstSeen(vm.ID())
		if !ok || now.Sub(firstSeen) < j.opts.ZeroUptimeCutoff {
			logger.Info("instance has 0 uptime, skipping for now")
			if !ok {
				j.setZeroUptimeFirstSeen(vm.ID(), now)
			}
			return nil
		}

		j.deleteZeroUptimeFirstSeen(vm.ID())
		logger.WithField("since_first_seen", time.Since(firstSeen)).Info("instance has had 0 uptime for more than timeout, destroying")
	} else {
		bootTime := vm.BootTime()

		if j.opts.SkipNoBootTime && bootTime == nil {
			logger.Info("instance has no boot time, skipping")
			return nil
		}

		if bootTime != nil {
			bootedAgo := now.UTC().Sub(*bootTime)
			logger.WithField("booted_ago", bootedAgo).Info("time since instance boot")
		}

		uptime := time.Duration(uptimeSecs) * time.Second
		if uptime < j.opts.Cutoff && vm.PoweredOn() {
			logger.WithField("uptime", uptime).WithField("powered_on", vm.PoweredOn()).Info("skipping instance")
			return nil
		}
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

func (j *Janitor) getZeroUptimeFirstSeen(id string) (time.Time, bool) {
	j.zeroUptimeFirstSeenMutex.Lock()
	defer j.zeroUptimeFirstSeenMutex.Unlock()
	firstSeen, ok := j.zeroUptimeFirstSeen[id]
	return firstSeen, ok
}

func (j *Janitor) setZeroUptimeFirstSeen(id string, now time.Time) {
	j.zeroUptimeFirstSeenMutex.Lock()
	defer j.zeroUptimeFirstSeenMutex.Unlock()
	j.zeroUptimeFirstSeen[id] = now
}

func (j *Janitor) deleteZeroUptimeFirstSeen(id string) {
	j.zeroUptimeFirstSeenMutex.Lock()
	defer j.zeroUptimeFirstSeenMutex.Unlock()
	delete(j.zeroUptimeFirstSeen, id)
}
