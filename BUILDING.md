# Developer Guide

## Prerequisites

- Download VDDK 7.0.2 libraries from [here](https://code.vmware.com/web/sdk/7.0/vddk) to
`<local path to velero-plugin-for-vsphere project>/.libs` and untar it.

## Building the plugin

To build the plugin, run

```bash
$ make
```

To build the container, run

```bash
$ make container
```

This builds an image named as `<REGISTRY>/velero-plugin-for-vsphere:<VERSION>`.
By default, the `VERSION`, i.e., tag, will be automatically generated in the format of,
`<git branch name>-<git commit>-<timestamp>`. For example, `master-ad4388f-11.Mar.2020.23.39.13`.
To push it to your own repo with your own tag, run

```bash
$ make container REGISTRY=<your-repo> VERSION=<your-tag>
```
or, just push it by running
```bash
$ make push REGISTRY=<your-repo> VERSION=<your-tag>
```

## Testing the plugin

To unit test the plugin, run

```bash
make test
```
