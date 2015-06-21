package vspherejanitor

import (
	"log"
	"net/url"
	"os"
	"time"

	"github.com/codegangsta/cli"
	"github.com/rcrowley/go-metrics"
	"github.com/rcrowley/go-metrics/librato"
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
	ratePerSecond := c.Int("rate-per-second")
	cleanupLoopSleep := c.Duration("cleanup-loop-sleep")

	janitor := NewJanitor(u, &JanitorOpts{
		Cutoff:        cutoffDuration,
		SkipDestroy:   skipDestroy,
		Concurrency:   concurrency,
		RatePerSecond: ratePerSecond,
	})

	if c.String("librato-email") != "" && c.String("librato-token") != "" && c.String("librato-source") != "" {
		log.Printf("starting librato metrics reporter")

		go librato.Librato(metrics.DefaultRegistry, time.Minute,
			c.String("librato-email"), c.String("librato-token"), c.String("librato-source"),
			[]float64{0.95}, time.Millisecond)

		if !c.Bool("silence-metrics") {
			go metrics.Log(metrics.DefaultRegistry, time.Minute,
				log.New(os.Stderr, "metrics: ", log.Lmicroseconds))
		}
	}

	for {
		for _, path := range paths {
			janitor.Cleanup(path)
		}

		if c.Bool("once") {
			log.Printf("finishing after one run")
			return
		}

		log.Printf("sleeping %s", cleanupLoopSleep)
		time.Sleep(cleanupLoopSleep)
	}
}
