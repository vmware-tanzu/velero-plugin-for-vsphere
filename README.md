# Velero Plugins for vSphere 

![Velero Plugin For vSphere CICD Pipeline](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/workflows/Velero%20Plugin%20For%20vSphere%20CICD%20Pipeline/badge.svg)

This repository contains Velero plugin for vSphere.

## Kinds of Plugin(s)

Velero plugin for vSphere currently supports the following kinds of Velero plugins:

- **Volume Snapshotter** - creates snapshots from volumes (during a backup) and volumes from snapshots (during a restore).


## Building the plugin

To build the plugin, run

```bash
$ make
```

To build the image, run

```bash
$ make container
```

This builds an image named as `<REGISTRY>/velero-plugin-for-vsphere:<VERSION>`.
By default, the `VERSION`, i.e., tag, will be automatically generated in the format of,
`<git branch name>-<git commit>-<timestamp>`. For example, `master-ad4388f-11.Mar.2020.23.39.13`.
If you want to eventually push it to your own repo with your own tag, run

```bash
$ make container REGISTRY=<your-repo> VERSION=<your-tag>
```
or, just push it by run
```bash
$ make push REGISTRY=<your-repo> VERSION=<your-tag>
```

## Installing the plugin

### Prerequisites


Velero - Version 1.3.0 or above
vSphere - Version 6.7U3 or above
vSphere CSI/CNS driver (compatible with 6.7U3+)
Kubernetes 1.14 or above

1. A working kubernetes cluster.
2. Velero is installed on the cluster based on the `Basic Install` guide, https://velero.io/docs/v1.2.0/basic-install/.

### Workflow

To deploy your image of Velero plugin for vSphere:

#### Velero v1.3.0 and later
1. Adding the plugin to Velero.
    ```bash
    velero plugin add <your-plugin-image>
    ```
2. Creating a VolumeSnapshotLocation with the provider being vSphere.
    ```bash
    velero snapshot-location create <your-volume-snapshot-location-name> --provider velero.io/vsphere
    ```
    
For additional information, please see the customized installation guide provided by Velero, 
https://velero.io/docs/v1.3.0/customize-installation/#install-an-additional-volume-snapshot-provider.

### Known Issues
1. In the vSphere CSI setup, if there is any network issue in the Velero pod. Run `kubectl patch deploy/velero -n velero --patch "$(cat deployment/patch-deployment-hostNetwork.yaml)"`.


## Using Velero with the plugin
Below are just basic example of use cases in Velero. For more use cases, please refer to https://velero.io/docs/.
### Backup
```bash
velero backup create <your-backup-name> \
    --include-namespaces=<your-app-namespace> \
    --snapshot-volumes \
    --volume-snapshot-locations <your-volume-snapshot-location-name>
```
If you don't like to specify the VolumeSnapshotLocation at each backup command,
you can do the following configuration.
1. Run `kubectl edit deployment/velero -n <velero-namespace>`
2. Edit the `spec.template.spec.containers[*].args` field under the velero container as below.
    ```yaml
    spec:
      template:
        spec:
          containers:
          - args:
            - server
            - --default-volume-snapshot-locations
            - velero.io/vsphere:<your-volume-snapshot-location-name>
    ```
### Restore
```bash
velero restore create --from-backup <your-backup-name>
```
