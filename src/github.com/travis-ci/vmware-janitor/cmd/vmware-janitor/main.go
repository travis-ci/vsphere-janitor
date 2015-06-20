package main

import (
	"os"

	"github.com/codegangsta/cli"
	"github.com/travis-ci/vmware-janitor"
)

func main() {
	app := cli.NewApp()
	app.Usage = "VMware cleanup thingy"
	app.Version = vmwarejanitor.VersionString
	app.Author = "Travis CI GmbH"
	app.Email = "contact+vmware-janitor@travis-ci.org"

	app.Flags = vmwarejanitor.Flags
	app.Action = vmwarejanitor.RunCleanup

	app.Run(os.Args)
}
