package vspherejanitor

import (
	"time"

	"github.com/codegangsta/cli"
)

var (
	Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "u, vsphere-url",
			Usage:  "URL of the vsphere server, including '/sdk' if applicable",
			EnvVar: "VSPHERE_JANITOR_VSPHERE_URL,VSPHERE_URL",
		},
		cli.StringSliceFlag{
			Name:   "p, vsphere-vm-paths",
			Usage:  "**REQUIRED**: Paths in inventory that contain VMs for cleanup",
			EnvVar: "VSPHERE_JANITOR_VSPHERE_VM_PATHS,VSPHERE_VM_PATHS",
		},
		cli.BoolFlag{
			Name:   "S, skip-destroy",
			Usage:  "Do not destroy VMs -- only power down",
			EnvVar: "VSPHERE_JANITOR_SKIP_DESTROY,SKIP_DESTROY",
		},
		cli.DurationFlag{
			Name:   "C, cutoff",
			Value:  2 * time.Hour,
			Usage:  "Max uptime cutoff",
			EnvVar: "VSPHERE_JANITOR_CUTOFF,CUTOFF",
		},
		cli.IntFlag{
			Name:   "c, concurrency",
			Usage:  "Concurrent cleanup goroutine count",
			EnvVar: "VSPHERE_JANITOR_CONCURRENCY,CONCURRENCY",
		},
		cli.BoolFlag{
			Name:   "O, once",
			Usage:  "Only run one cleanup",
			EnvVar: "VSPHERE_JANITOR_ONCE,ONCE",
		},
		cli.DurationFlag{
			Name:   "s, cleanup-loop-sleep",
			Value:  1 * time.Minute,
			Usage:  "Sleep interval between cleaning up all paths",
			EnvVar: "VSPHERE_JANITOR_CLEANUP_LOOP_SLEEP,CLEANUP_LOOP_SLEEP",
		},
		cli.IntFlag{
			Name:   "R, rate-per-second",
			Value:  5,
			Usage:  "Rate limit max vms handled per second",
			EnvVar: "VSPHERE_JANITOR_RATE_PER_SECOND,RATE_PER_SECOND",
		},
	}
)
