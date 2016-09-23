package main

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"time"

	librato "github.com/mihasya/go-metrics-librato"
	metrics "github.com/rcrowley/go-metrics"
	"github.com/travis-ci/vsphere-janitor"
	"github.com/urfave/cli"
)

var (
	// VersionString is the git describe version set at build time
	VersionString = "?"
	// RevisionString is the git revision set at build time
	RevisionString = "?"
	// GeneratedString is the build date set at build time
	GeneratedString = "?"
)

func init() {
	cli.VersionPrinter = customVersionPrinter
	os.Setenv("VERSION", VersionString)
	os.Setenv("REVISION", RevisionString)
	os.Setenv("GENERATED", GeneratedString)
}

func customVersionPrinter(c *cli.Context) {
	fmt.Printf("%v v=%v rev=%v d=%v\n", c.App.Name, VersionString, RevisionString, GeneratedString)
}

func main() {
	app := cli.NewApp()
	app.Usage = "VMware vSphere cleanup thingy"
	app.Version = VersionString
	app.Author = "Travis CI GmbH"
	app.Email = "contact+vsphere-janitor@travis-ci.org"

	app.Flags = Flags
	app.Action = mainAction

	app.Run(os.Args)
}

func mainAction(c *cli.Context) error {
	u, err := url.Parse(c.String("vsphere-url"))
	if err != nil {
		log.Fatal(err)
	}

	paths := c.StringSlice("vsphere-vm-paths")
	if len(paths) == 0 {
		log.Fatal("missing vsphere vm paths")
	}

	cleanupLoopSleep := c.Duration("cleanup-loop-sleep")

	janitor := vspherejanitor.NewJanitor(u, &vspherejanitor.JanitorOpts{
		Cutoff:         c.Duration("cutoff"),
		SkipDestroy:    c.Bool("skip-destroy"),
		Concurrency:    c.Int("concurrency"),
		RatePerSecond:  c.Int("rate-per-second"),
		SkipZeroUptime: c.BoolT("skip-zero-uptime"),
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
			break
		}

		log.Printf("sleeping %s", cleanupLoopSleep)
		time.Sleep(cleanupLoopSleep)
	}

	return nil
}
