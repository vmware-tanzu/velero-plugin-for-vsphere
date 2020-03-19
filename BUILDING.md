
## Building the plugin

The Velero Plugin for vSphere is implemented 
The Velero Plugin for vSphere relies on astrolabe and gvddk.  Before building the plugin, 

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

