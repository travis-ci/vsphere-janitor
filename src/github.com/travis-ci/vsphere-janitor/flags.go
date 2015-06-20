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
			EnvVar: "VSPHERE_JANITOR_VSPHERE_URL, VSPHERE_URL",
		},
		cli.StringFlag{
			Name:   "p, vsphere-vm-path",
			Usage:  "**REQUIRED**: Path in inventory that contains VMs for cleanup",
			EnvVar: "VSPHERE_JANITOR_VSPHERE_VM_PATH, VSPHERE_VM_PATH",
		},
		cli.BoolFlag{
			Name:   "S, skip-destroy",
			Usage:  "Do not destroy VMs -- only power down",
			EnvVar: "VSPHERE_JANITOR_SKIP_DESTROY, SKIP_DESTROY",
		},
		cli.DurationFlag{
			Name:   "C, cutoff",
			Value:  2 * time.Hour,
			Usage:  "Max uptime cutoff",
			EnvVar: "VSPHERE_JANITOR_CUTOFF, CUTOFF",
		},
		cli.IntFlag{
			Name:   "c, concurrency",
			Usage:  "Concurrent cleanup goroutine count",
			EnvVar: "VSPHERE_JANITOR_CONCURRENCY, CONCURRENCY",
		},
	}
)
