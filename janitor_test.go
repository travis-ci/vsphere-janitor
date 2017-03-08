package vspherejanitor_test

import (
	"context"
	"testing"
	"time"

	vspherejanitor "github.com/travis-ci/vsphere-janitor"
	"github.com/travis-ci/vsphere-janitor/mock"
)

type testCase struct {
	now        time.Time
	config     *vspherejanitor.JanitorOpts
	vms        []*mock.VMData
	poweredOff []string
	destroyed  []string
}

var aTime = time.Date(2016, 01, 15, 12, 0, 0, 0, time.UTC)

var janitorTestCases []testCase = []testCase{
	{
		now: aTime,
		config: &vspherejanitor.JanitorOpts{
			Cutoff:           time.Hour,
			ZeroUptimeCutoff: time.Minute,
			SkipDestroy:      false,
			Concurrency:      1,
			RatePerSecond:    100,
			SkipNoBootTime:   true,
		},
		vms: []*mock.VMData{
			{
				Name:      "old-powered-on",
				Uptime:    2 * time.Hour,
				BootTime:  timePointer(aTime.Add(-2 * time.Hour)),
				PoweredOn: true,
			},
			{
				Name:      "new-powered-on",
				Uptime:    30 * time.Minute,
				BootTime:  timePointer(aTime.Add(-30 * time.Minute)),
				PoweredOn: true,
			},
			{
				Name:      "powered-off",
				Uptime:    0,
				BootTime:  nil,
				PoweredOn: false,
			},
		},
	},
}

func TestJanitor(t *testing.T) {
	for _, c := range janitorTestCases {
		vmLister := mock.NewVMLister(map[string][]*mock.VMData{
			"/": c.vms,
		})

		janitor := vspherejanitor.NewJanitor(vmLister, c.config)
		err := janitor.Cleanup(context.TODO(), "/", c.now)
		assertOk(t, "janitor.Cleanup(/)", err)

		assertEqual(t, `PoweredOff("/", "old-powered-on")`, true, vmLister.PoweredOff("/", "old-powered-on"))
		assertEqual(t, `Destroyed("/", "old-powered-on")`, true, vmLister.Destroyed("/", "old-powered-on"))
		assertEqual(t, `PoweredOff("/", "new-powered-on")`, true, vmLister.PoweredOff("/", "old-powered-on"))
		assertEqual(t, `Destroyed("/", "new-powered-on")`, true, vmLister.Destroyed("/", "old-powered-on"))
	}
}

func assertEqual(tb testing.TB, name string, expected, actual interface{}) {
	if expected != actual {
		tb.Errorf("%s: expected %v, but was %v", name, expected, actual)
	}
}

func assertOk(tb testing.TB, name string, err error) {
	if err != nil {
		tb.Fatalf("%s: returned error: %v", name, err)
	}
}

func assertError(tb testing.TB, name string, err error) {
	if err == nil {
		tb.Fatalf("%s: didn't return error", name)
	}
}

func timePointer(t time.Time) *time.Time {
	tp := new(time.Time)
	*tp = t
	return tp
}
