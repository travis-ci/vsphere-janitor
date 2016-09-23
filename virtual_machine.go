package vspherejanitor

import (
	"context"
	"time"
)

type VMLister interface {
	ListVMs(ctx context.Context, path string) ([]VirtualMachine, error)
}

type VirtualMachine interface {
	Name() string
	Uptime() time.Duration
	BootTime() *time.Time
	PoweredOn() bool
	PowerOff(context.Context) error
	Destroy(context.Context) error
}
