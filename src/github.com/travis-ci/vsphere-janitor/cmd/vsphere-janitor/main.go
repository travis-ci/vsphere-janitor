package main

import (
	"os"

	"github.com/codegangsta/cli"
	"github.com/travis-ci/vsphere-janitor"
)

func main() {
	app := cli.NewApp()
	app.Usage = "VMware vSphere cleanup thingy"
	app.Version = vspherejanitor.VersionString
	app.Author = "Travis CI GmbH"
	app.Email = "contact+vsphere-janitor@travis-ci.org"

	app.Flags = vspherejanitor.Flags
	app.Action = vspherejanitor.RunCleanup

	app.Run(os.Args)
}
