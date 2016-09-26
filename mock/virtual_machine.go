package mock

import (
	"context"
	"errors"
	"sync"
	"time"

	vspherejanitor "github.com/travis-ci/vsphere-janitor"
)

type VMLister struct {
	VMData map[string][]*VMData

	mutex      sync.Mutex
	poweredOff map[string][]string
	destroyed  map[string][]string
}

func NewVMLister(data map[string][]*VMData) *VMLister {
	poweredOff := make(map[string][]string, len(data))
	destroyed := make(map[string][]string, len(data))
	for path := range data {
		poweredOff[path] = make([]string, 0)
		destroyed[path] = make([]string, 0)
	}

	return &VMLister{
		VMData:     data,
		poweredOff: poweredOff,
		destroyed:  destroyed,
	}
}

func (vl *VMLister) PoweredOff(path, searchName string) bool {
	vmNames, ok := vl.poweredOff[path]
	if !ok {
		return false
	}

	for _, name := range vmNames {
		if name == searchName {
			return true
		}
	}

	return false
}

func (vl *VMLister) powerOff(path, name string) {
	vl.mutex.Lock()
	defer vl.mutex.Unlock()

	vl.poweredOff[path] = append(vl.poweredOff[path], name)
}

func (vl *VMLister) Destroyed(path, searchName string) bool {
	vmNames, ok := vl.destroyed[path]
	if !ok {
		return false
	}

	for _, name := range vmNames {
		if name == searchName {
			return true
		}
	}

	return false
}

func (vl *VMLister) destroy(path, name string) {
	vl.mutex.Lock()
	defer vl.mutex.Unlock()

	vl.destroyed[path] = append(vl.destroyed[path], name)
}

func (vl *VMLister) ListVMs(ctx context.Context, path string) ([]vspherejanitor.VirtualMachine, error) {
	vmData, ok := vl.VMData[path]
	if !ok {
		return nil, errors.New("no such path")
	}

	vms := make([]vspherejanitor.VirtualMachine, 0, len(vmData))

	for _, vm := range vmData {
		vms = append(vms, &VirtualMachine{lister: vl, path: path, data: vm})
	}

	return vms, nil
}

type VMData struct {
	Name      string
	Uptime    time.Duration
	BootTime  *time.Time
	PoweredOn bool
}

type VirtualMachine struct {
	lister *VMLister
	path   string
	data   *VMData
}

func (vm *VirtualMachine) Name() string {
	return vm.data.Name
}

func (vm *VirtualMachine) Uptime() time.Duration {
	return vm.data.Uptime
}

func (vm *VirtualMachine) BootTime() *time.Time {
	return vm.data.BootTime
}

func (vm *VirtualMachine) PoweredOn() bool {
	return vm.data.PoweredOn
}

func (vm *VirtualMachine) PowerOff(context.Context) error {
	vm.lister.powerOff(vm.path, vm.data.Name)

	return nil
}

func (vm *VirtualMachine) Destroy(context.Context) error {
	vm.lister.destroy(vm.path, vm.data.Name)

	return nil
}
