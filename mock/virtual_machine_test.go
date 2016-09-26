package mock

import (
	"context"
	"fmt"
	"testing"
	"time"
)

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

func TestVMLister(t *testing.T) {
	now := time.Now()
	lister := NewVMLister(map[string][]*VMData{
		"/empty": []*VMData{},
		"/one": []*VMData{
			{
				Name:      "test-vm",
				Uptime:    time.Minute,
				BootTime:  &now,
				PoweredOn: true,
			},
		},
	})

	vms, err := lister.ListVMs(context.TODO(), "/empty")
	assertOk(t, "ListVMs(/empty)", err)
	assertEqual(t, "ListVMs(/empty) len(vms)", 0, len(vms))

	vms, err = lister.ListVMs(context.TODO(), "/does-not-exist")
	assertError(t, "ListVMs(/does-not-exist)", err)

	vms, err = lister.ListVMs(context.TODO(), "/one")
	assertOk(t, "ListVMs(/one)", err)
	assertEqual(t, "ListVMs(/one) len(vms)", 1, len(vms))
	assertEqual(t, "ListVMs(/one)[0].Name()", "test-vm", vms[0].Name())
	assertEqual(t, "ListVMs(/one)[0].Uptime()", time.Minute, vms[0].Uptime())
	assertEqual(t, "ListVMs(/one)[0].PoweredOn()", true, vms[0].PoweredOn())
	if !vms[0].BootTime().Equal(now) {
		t.Errorf("expected %v, but was %v", now, vms[0].BootTime())
	}
}

func TestVirtualMachinePowerOff(t *testing.T) {
	now := time.Now()
	lister := NewVMLister(map[string][]*VMData{
		"/one": []*VMData{
			{
				Name:      "test-vm",
				Uptime:    time.Minute,
				BootTime:  &now,
				PoweredOn: true,
			},
		},
	})

	poweredOff := lister.PoweredOff("/does-not-exist", "foo")
	assertEqual(t, fmt.Sprintf("lister.PoweredOff(%q, %q)", "/does-not-exist", "foo"), false, poweredOff)

	vms, err := lister.ListVMs(context.TODO(), "/one")
	assertOk(t, "ListVMs(/one)", err)
	assertEqual(t, "ListVMs(/one) len(vms)", 1, len(vms))

	poweredOff = lister.PoweredOff("/one", vms[0].Name())
	assertEqual(t, fmt.Sprintf("lister.PoweredOff(%q, %q)", "/one", vms[0].Name()), false, poweredOff)

	err = vms[0].PowerOff(context.TODO())
	assertOk(t, "vm.PowerOff()", err)

	poweredOff = lister.PoweredOff("/one", vms[0].Name())
	assertEqual(t, fmt.Sprintf("lister.PoweredOff(%q, %q)", "/one", vms[0].Name()), true, poweredOff)
}

func TestVirtualMachineDestroy(t *testing.T) {
	now := time.Now()
	lister := NewVMLister(map[string][]*VMData{
		"/one": []*VMData{
			{
				Name:      "test-vm",
				Uptime:    time.Minute,
				BootTime:  &now,
				PoweredOn: true,
			},
		},
	})

	destroyed := lister.Destroyed("/does-not-exist", "foo")
	assertEqual(t, fmt.Sprintf("lister.Destroyed(%q, %q)", "/does-not-exist", "foo"), false, destroyed)

	vms, err := lister.ListVMs(context.TODO(), "/one")
	assertOk(t, "ListVMs(/one)", err)
	assertEqual(t, "ListVMs(/one) len(vms)", 1, len(vms))

	destroyed = lister.Destroyed("/one", vms[0].Name())
	assertEqual(t, fmt.Sprintf("lister.Destroyed(%q, %q)", "/one", vms[0].Name()), false, destroyed)

	err = vms[0].Destroy(context.TODO())
	assertOk(t, "vm.Destroy()", err)

	destroyed = lister.Destroyed("/one", vms[0].Name())
	assertEqual(t, fmt.Sprintf("lister.Destroyed(%q, %q)", "/one", vms[0].Name()), true, destroyed)
}
