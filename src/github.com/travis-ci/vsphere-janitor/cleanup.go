package vspherejanitor

import (
	"log"
	"net/url"
	"time"

	"github.com/codegangsta/cli"
)

func RunCleanup(c *cli.Context) {
	u, err := url.Parse(c.String("vsphere-url"))
	if err != nil {
		log.Fatal(err)
	}

	paths := c.StringSlice("vsphere-vm-paths")
	if len(paths) == 0 {
		log.Fatal("missing vsphere vm paths")
	}

	skipDestroy := c.Bool("skip-destroy")
	cutoffDuration := c.Duration("cutoff")
	concurrency := c.Int("concurrency")

	janitor := NewJanitor(u, &JanitorOpts{
		Cutoff:      cutoffDuration,
		SkipDestroy: skipDestroy,
		Concurrency: concurrency,
	})

	for {
		for _, path := range paths {
			janitor.Cleanup(path)
		}

		if c.Bool("once") {
			return
		}

		time.Sleep(c.Duration("cleanup-loop-sleep"))
	}
}
