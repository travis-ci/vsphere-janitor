package main

import (
	"fmt"
	"os"

	"github.com/codegangsta/cli"
	"github.com/travis-ci/vsphere-janitor"
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

	app.Flags = vspherejanitor.Flags
	app.Action = vspherejanitor.RunCleanup

	app.Run(os.Args)
}
