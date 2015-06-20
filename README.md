# vSphere Janitor

A little tool used for cleaning up leftover ("stale") VMs in vSphere.

## configuration

Example configuration is available in the [example.env file](./example.env).

## running via cron

Check out the [example script](./cron-example) meant to be run via `cron` with
env var configuration from `/etc/default/{name-of-cron-script}`.
