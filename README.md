# Velero Plugin for vSphere 

![Velero Plugin For vSphere CICD Pipeline](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/workflows/Velero%20Plugin%20For%20vSphere%20CICD%20Pipeline/badge.svg)

## Overview
This repository contains the Velero Plugin for vSphere.  This plugin is a volume snapshotter plugin that provides crash-consistent snapshots of vSphere block volumes and backup of volume data into S3 compatible storage.

## Compatibility
* Velero - Version 1.3.2 or above
* vSphere - Version 6.7U3 or above
* vSphere CSI/CNS driver 1.0.2 or above
* Kubernetes 1.14 or above (note: the Velero Plug-in for vSphere does not support Guest or Supervisor clusters on vSphere yet)


## Installing the plugin
* Install Velero - please see the Velero documentation https://velero.io/docs/v1.3.1/basic-install
* [Install the AWS plugin](#Install-the-aws-plugin)
* [Install the plugin](#Install-the-plugin)
* [Create a VolumeSnapshotLocation](Create-a-VolumeSnapshotLocation)
* [Backup using the plugin](Backup-using-the-plugin)

## Install the AWS plugin
Volume backups are stored in an S3 bucket.  Currently these are stored in the same bucket configured for
the AWS plugin.  Before installing the vSphere plugin, please install and configure the AWS plugin 
(https://github.com/vmware-tanzu/velero-plugin-for-aws/blob/master/README.md)

## Install the plugin
```bash
velero plugin add <plugin-image>
```

For Version 1.0.2 the command is

```bash
velero plugin add vsphereveleroplugin/velero-plugin-for-vsphere:1.0.2
```

## Create a VolumeSnapshotLocation

The VolumeSnapshotLocation will be used to specify the use of the Velero Plug-in for vSphere.

```bash
velero snapshot-location create <snapshot location name> --provider velero.io/vsphere
```

For our examples in this document, we name the VolumeSnapshotLocation vsl-vsphere.

```bash
velero snapshot-location create vsl-vsphere --provider velero.io/vsphere
```

## Backup using the plugin

To use the plugin, run a Velero backup and specify the --snapshot-volumes flag and specify the VolumeSnapshotLocation
you set above with the --volume-snapshot-locations flag

```bash
velero backup create my-backup --include-namespaces=my-namespace --snapshot-volumes --volume-snapshot-locations vsl-vsphere
```

If attempting to backup the entire cluster please exclude the upload and download crds by using the --exclude-resources flag

```bash
velero backup create my-backup --snapshot-volumes --volume-snapshot-locations vsl-vsphere --exclude-resources uploads.veleroplugin.io,downloads.veleroplugin.io,uploads.veleroplugin.io,downloads.veleroplugin.io
```

Your backup will complete after the local snapshots have completed and your Kubernetes metadata has been uploaded to the object
store specified.  At this point, all of the data may not have been uploaded to your S3 object store.  Data movement happens in the
background and may take a significant amount of time to complete.

## Monitoring data upload progress

For each volume snapshot that is uploaded to S3, an uploads.veleroplugin.io customer resource is generated.  These records contain the current state of an upload request.  You can list out current requests with

```bash
kubectl get -n <velero namespace> uploads.veleroplugin.io -o yaml
```

An upload record looks like this;

```
- apiVersion: veleroplugin.io/v1
  kind: Upload
  metadata:
    creationTimestamp: "2020-03-16T21:12:48Z"
    generation: 1
    name: upload-bcc6e06c-8d2f-4e19-b157-0dbd1ef9fcb2
    namespace: velero
    resourceVersion: "2173887"
    selfLink: /apis/veleroplugin.io/v1/namespaces/velero/uploads/upload-bcc6e06c-8d2f-4e19-b157-0dbd1ef9fcb2
    uid: e3a2a3ee-67ca-11ea-af6d-005056a56c10
  spec:
    backupTimestamp: "2020-03-16T21:12:48Z"
    snapshotID: ivd:bb3c52ce-012b-4cb0-86ea-7324145b254e:bcc6e06c-8d2f-4e19-b157-0dbd1ef9fcb2
  status:
    phase: New
    progress: {}
```

Uploads have five phases (status/phase in YAML):
* New - not processed yet
* InProgress - data being moved
* Completed - data moved successfully
* UploadError - data movement upload failed
* CleanupFailed - delete snapshot failed, this case will not retry

UploadError uploads will be periodically retried.  At that point their phase will return to InProgress.  After an upload has been 
successfully completed, its record will remain for a period of time and eventually be removed.

## Restore
In order to restore you must have a working Kubernetes cluster on vSphere and have Velero and the Velero Plugin for vSphere installed
and configured.  There are no special options to the plugin required for restore.  The basic restore command is:

```bash
velero restore create --from-backup <your-backup-name>
```

If attempting to restore an entire cluster from backup then exclude the upload and download crds by using the --exclude-resources flag

```bash
velero restore create --from-backup <my-full-cluster-backup-name> --exclude-resources uploads.veleroplugin.io,downloads.veleroplugin.io,uploads.veleroplugin.io,downloads.veleroplugin.io
```

Please refer to the Velero documentation for usage and additional restore options.

## Setting a default VolumeSnapshotLocation
If you don't want to specify the VolumeSnapshotLocation for each backup command,
follow these steps to set a default VolumeSnapshotLocation.
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
## Uninstall the plugin
To uninstall the plugin, run
```bash
velero plugin remove <plugin-image>
```
to remove the plugin from the Velero deployment. To finish the cleanup, delete the Data Manager daemonset and related CRDs.
```bash
kubectl -n velero delete daemonset.apps/datamgr-for-vsphere-plugin
kubectl delete crds uploads.veleroplugin.io downloads.veleroplugin.io
```

## S3 data
Your volume data is stored in the Velero bucket with prefixes beginning with *plugins/vsphere-astrolabe-repo*
Velero versions prior to 1.3.2 will fail when using a bucket that has objects with the *plugins* prefix.  If you have
multiple Velero instances sharing a common bucket, please be sure to upgrade all of the to 1.3.2 before making any
backups with the vSphere plugin 

## Backup vSphere CNS File Volumes
The Velero Plugin for vSphere is designed to backup vSphere CNS block volumes.  vSphere CNS
file volumes should be backed up with the [Velero Restic Integration](https://velero.io/docs/v1.4/restic/).  File volumes must be annotated for Restic backup.  Block and file volumes may be backed up together.

To use Restic backup for file volumes, please use the `--use-restic` flag to `velero install` command when 
installing Velero.  Annotate all PVs backed by vSphere CNS file volumes before running any `velero backup`
commands by using the following command for each pod that contains one or more file folumes to
back up:
```
kubectl -n YOUR_POD_NAMESPACE annotate pod/YOUR_POD_NAME backup.velero.io/backup-volumes=FILE_VOLUME_NAME_1,FILE_VOLUME_NAME_2,...
```

## Current release:

 *[CHANGELOG-1.0.2.md][1]

[1]: https://github.com/vmware-tanzu/velero-plugin-for-vsphere/blob/master/changelogs/CHANGELOG-1.0.2.md

