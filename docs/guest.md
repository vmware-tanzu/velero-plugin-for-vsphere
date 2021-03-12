# Tanzu Kubernetes Grid Service

This document discusses the velero vSphere plugin installation process in a **Tanzu Kubernetes Grid Service** environment aka TKG Guest Clusters. For more information on the Tanzu Kubernetes Grid Service refer to the following [documentation](https://docs.vmware.com/en/VMware-Tanzu-Kubernetes-Grid/index.html).

## Compatibility

| vSphere/ESXi Version                   | vSphere CSI Version          | Kubernetes Version | Velero Version    | Velero Plugin for vSphere Version |
|----------------------------------------|------------------------------|--------------------|-------------------|-----------------------------------|
| vSphere 7.0(U1c/P02)/ESXi 7.0(U1c/P02) | CSI driver bundled with TKGS | v1.16-v1.19        | v1.5.1 and higher | v1.1.0 and higher                 |

## Prerequisites

### Install Velero plugin for vSphere in Supervisor cluster

Prior to the installation of the **Velero plugin for vSphere** in Guest cluster it is necessary to have Velero and **Velero plugin for vSphere** installed in the Supervisor Cluster. Please refer to [Supervisor Cluster Velero vSphere Plugin](supervisor.md) installation guide.

### Install the Data Manager

The Data Manager Virtual Machine also needs to be installed to enable TKGS backups and restores. Please refer to [DataManager Documentation](supervisor-datamgr.md) for details on how to deploy the Data Manager.

## Installation Steps

1. [Install Velero](https://velero.io/docs/v1.5/basic-install/)
2. [Install Object Storage Plugin](#install-object-storage-plugin)
3. [Install Velero Plugin for vSphere](#install-velero-plugin-for-vsphere)

### Install Object Storage Plugin

Volume backups are stored in an object store bucket. They are stored in the same bucket configured for the object storage plugin of Velero. Before installing the vSphere plugin, a Velero object storage plugin is required.

Currently, only AWS plugin is supported and compatible with vSphere plugin. Please refer to [velero-plugin-for-aws](https://github.com/vmware-tanzu/velero-plugin-for-aws/blob/master/README.md) for more details about using **AWS S3** as the object store for backups. S3-compatible object stores, e.g, **MinIO**, are also supported via AWS plugin. Please refer to [install with MinIO](https://velero.io/docs/v1.5/contributions/minio/).

### Install Velero Plugin for vSphere

```bash
velero plugin add <plugin-image>
```

For Version 1.1.0 the command is

```bash
velero plugin add vsphereveleroplugin/velero-plugin-for-vsphere:1.1.0
```

* The installation of Velero vSphere Plugin in Guest Cluster may take over ten minutes to complete, this is primarily because the plugin waits for the Velero app operator to write the Secret into a predefined namespace.
* The installation may hang forever if Velero is not installed in the Supervisor Cluster.

## Uninstall

To uninstall the plugin, run the following command to remove the **InitContainer** of velero-plugin-for-vsphere from the Velero deployment first.

```bash
velero plugin remove <plugin image>
```

To finish the cleanup, delete the Backup Driver deployment, and their related CRDs.

```bash
kubectl -n velero delete deployment.apps/backup-driver

kubectl delete crds backuprepositories.backupdriver.cnsdp.vmware.com \
                    backuprepositoryclaims.backupdriver.cnsdp.vmware.com \
                    clonefromsnapshots.backupdriver.cnsdp.vmware.com \
                    deletesnapshots.backupdriver.cnsdp.vmware.com \
                    snapshots.backupdriver.cnsdp.vmware.com

kubectl delete crds uploads.datamover.cnsdp.vmware.com downloads.datamover.cnsdp.vmware.com
```

## Backup

### Backup vSphere CNS Block Volumes

Below is an example command of Velero backup.

```bash
velero backup create <backup name> --include-namespaces=my-namespace
```

For more backup options, please refer to [Velero Document](https://velero.io/docs/v1.5/).

Velero backup will be marked as `Completed` after all local snapshots have been taken and Kubernetes metadata, **except** volume snapshots, has been uploaded to the object store. At this point, async data movement tasks, i.e., the upload of volume snapshot, are still happening in the background and may take some time to complete. We can check the status of volume snapshot by monitoring [Snapshot](#snapshots) Custom Resources (CRs).

### Snapshots

For each volume snapshot, a Snapshot CR will be created in the same namespace as the PVC that is snapshotted. We can get all Snapshots in PVC namespace by running the following command.

```bash
kubectl get -n <pvc namespace> snapshot
```

Here is an example Snapshot CR in YAML.

```bash
apiVersion: backupdriver.cnsdp.vmware.com/v1alpha1
kind: Snapshot
metadata:
  creationTimestamp: "2020-12-17T22:34:17Z"
  generation: 1
  labels:
    velero.io/backup-name: test-dp-1217
    velero.io/exclude-from-backup: "true"
  managedFields: ...
  name: snap-20d44de3-bd67-4b0f-be4b-278c62676276
  namespace: test-ns-mctwohb
  resourceVersion: "2515406"
  selfLink: /apis/backupdriver.cnsdp.vmware.com/v1alpha1/namespaces/test-ns-mctwohb/snapshots/snap-20d44de3-bd67-4b0f-be4b-278c62676276
  uid: 6ead6881-be4f-45fa-b01d-0505d540615f
spec:
  backupRepository: br-fa2b8bec-e99b-407a-9f95-dce31ff2bca6
  resourceHandle:
    apiGroup: ""
    kind: PersistentVolumeClaim
    name: etcd0-pv-claim
status:
  completionTimestamp: "2020-12-17T22:37:42Z"
  metadata: CqEKCg5ldGNkMC1wdi1jbGFpbRIAGg90ZXN0LW5zLW1jdHdvaGIiSC9hcGkvdjEvbmFtZXNwYWNlcy90ZXN0LW5zLW1jdHdvaGIvcGVyc2lzdGVudHZvbHVtZWNsYWltcy9ldGNkMC1wdi1jbGFpbSokODA4NjhmOGItNGZlYy00YmQyLWIyNDYtZTUyYzY2ZGI1NTNhMgQyMzk4OABCCAi5iLv+BRAAYrwCCjBrdWJlY3RsLmt1YmVybmV0ZXMuaW8vbGFzdC1hcHBsaWVkLWNvbmZpZ3VyYXRpb24ShwJ7ImFwaVZlcnNpb24iOiJ2MSIsImtpbmQiOiJQZXJzaXN0ZW50Vm9sdW1lQ2xhaW0iLCJtZXRhZGF0YSI6eyJhbm5vdGF0aW9ucyI6e30sIm5hbWUiOiJldGNkMC1wdi1jbGFpbSIsIm5hbWVzcGFjZSI6InRlc3QtbnMtbWN0d29oYiJ9LCJzcGVjIjp7ImFjY2Vzc01vZGVzIjpbIlJlYWRXcml0ZU9uY2UiXSwicmVzb3VyY2VzIjp7InJlcXVlc3RzIjp7InN0b3JhZ2UiOiIxR2kifX0sInN0b3JhZ2VDbGFzc05hbWUiOiJraWJpc2hpaS1zdG9yYWdlLWNsYXNzIn19CmImCh9wdi5rdWJlcm5ldGVzLmlvL2JpbmQtY29tcGxldGVkEgN5ZXNiKwokcHYua3ViZXJuZXRlcy5pby9ib3VuZC1ieS1jb250cm9sbGVyEgN5ZXNiRwotdm9sdW1lLmJldGEua3ViZXJuZXRlcy5pby9zdG9yYWdlLXByb3Zpc2lvbmVyEhZjc2kudnNwaGVyZS52bXdhcmUuY29tchxrdWJlcm5ldGVzLmlvL3B2Yy1wcm90ZWN0aW9uegCKAZ8CChlrdWJlY3RsLWNsaWVudC1zaWRlLWFwcGx5EgZVcGRhdGUaAnYxIggIuYi7/gUQADIIRmllbGRzVjE64QEK3gF7ImY6bWV0YWRhdGEiOnsiZjphbm5vdGF0aW9ucyI6eyIuIjp7fSwiZjprdWJlY3RsLmt1YmVybmV0ZXMuaW8vbGFzdC1hcHBsaWVkLWNvbmZpZ3VyYXRpb24iOnt9fX0sImY6c3BlYyI6eyJmOmFjY2Vzc01vZGVzIjp7fSwiZjpyZXNvdXJjZXMiOnsiZjpyZXF1ZXN0cyI6eyIuIjp7fSwiZjpzdG9yYWdlIjp7fX19LCJmOnN0b3JhZ2VDbGFzc05hbWUiOnt9LCJmOnZvbHVtZU1vZGUiOnt9fX2KAdgCChdrdWJlLWNvbnRyb2xsZXItbWFuYWdlchIGVXBkYXRlGgJ2MSIICPKIu/4FEAAyCEZpZWxkc1YxOpwCCpkCeyJmOm1ldGFkYXRhIjp7ImY6YW5ub3RhdGlvbnMiOnsiZjpwdi5rdWJlcm5ldGVzLmlvL2JpbmQtY29tcGxldGVkIjp7fSwiZjpwdi5rdWJlcm5ldGVzLmlvL2JvdW5kLWJ5LWNvbnRyb2xsZXIiOnt9LCJmOnZvbHVtZS5iZXRhLmt1YmVybmV0ZXMuaW8vc3RvcmFnZS1wcm92aXNpb25lciI6e319fSwiZjpzcGVjIjp7ImY6dm9sdW1lTmFtZSI6e319LCJmOnN0YXR1cyI6eyJmOmFjY2Vzc01vZGVzIjp7fSwiZjpjYXBhY2l0eSI6eyIuIjp7fSwiZjpzdG9yYWdlIjp7fX0sImY6cGhhc2UiOnt9fX0ScQoNUmVhZFdyaXRlT25jZRISEhAKB3N0b3JhZ2USBQoDMUdpGihwdmMtODA4NjhmOGItNGZlYy00YmQyLWIyNDYtZTUyYzY2ZGI1NTNhKhZraWJpc2hpaS1zdG9yYWdlLWNsYXNzMgpGaWxlc3lzdGVtGigKBUJvdW5kEg1SZWFkV3JpdGVPbmNlGhAKB3N0b3JhZ2USBQoDMUdp
  phase: Uploaded
  progress: {}
  snapshotID: pvc:test-ns-mctwohb/etcd0-pv-claim:aXZkOmQ4NzQwNDE0LTZmZjMtNDZjNi05YjM4LTllNDdlNDBhZmIwNjozZDU3MDU3ZC1lMDcwLTRkNDktYmJlNC0xY2Y0YWYxM2NiNjQ
  svcSnapshotName: "snap-30d44de3-cd67-5b0f-ce4b-378c62676276"
```

Snapshot CRD has a number of phases for the `.status.phase` field:

* New: not processed yet
* Snapshotted: local snapshot was taken
* SnapshotFailed: local snapshot was failed
* Uploading: the snapshot is being uploaded
* Uploaded: the snapshot is uploaded
* UploadFailed: the snapshot is failed to be uploaded
* Canceling: the upload of snapshot is being cancelled
* Canceled: the upload of snapshot is cancelled
* CleanupAfterUploadFailed: the Cleanup of local snapshot after the upload of snapshot was failed

## Restore

Below is an example command of Velero restore.

```bash
velero restore create --from-backup <your-backup-name>
```

Velero restore will be marked as `Completed` when volume snapshots and other Kubernetes metadata have been successfully
restored to the current cluster. At this point, all tasks of vSphere plugin related to this restore are completed as well.
There are no any async data movement tasks behind the scene as that in the case of Velero backup.

Before Velero restore is `Completed`, we can the status of volume restore by monitoring [CloneFromSnapshots](#clonefromsnapshots) CRs
as below.

* Restore of a Backup created in Vanilla setup to a Guest Cluster is not supported.
* Restore of a Backup created in Supervisor Cluster to a Guest Cluster is not supported.
* Restore of a Backup created in Guest Cluster to a Vanilla setup is not supported.
* Restore of a Backup created in Guest Cluster to a Supervisor Cluster is not supported.

### CloneFromSnapshots

To restore from each volume snapshot, a ``CloneFromSnapshot`` Custom Resource (CR) will be created in the same namespace as the PVC that is originally snapshotted. We can get all CloneFromSnapshots in PVC namespace by running the following command.

```bash
kubectl -n <pvc namespace> get clonefromsnapshot
```

Here is an example ```CloneFromSnapshot``` CR in YAML.

```bash
apiVersion: backupdriver.cnsdp.vmware.com/v1alpha1
kind: CloneFromSnapshot
metadata:
  creationTimestamp: "2020-12-17T23:32:40Z"
  generation: 1
  labels:
    velero.io/exclude-from-backup: "true"
  managedFields: ...
  name: fa218f2d-e6b3-48ab-a2ce-4820bfe2e16f
  namespace: test-ns-mctwohb
  resourceVersion: "2525829"
  selfLink: /apis/backupdriver.cnsdp.vmware.com/v1alpha1/namespaces/test-ns-mctwohb/clonefromsnapshots/fa218f2d-e6b3-48ab-a2ce-4820bfe2e16f
  uid: 94b49e3e-26ae-4534-ad0f-3ee89a8d0bc4
spec:
  apiGroup: ""
  backupRepository: br-fa2b8bec-e99b-407a-9f95-dce31ff2bca6
  cloneCancel: false
  kind: PersistentVolumeClaim
  metadata: CpIHCiNraWJpc2hpaS1kYXRhLWtpYmlzaGlpLWRlcGxveW1lbnQtMBIAGg90ZXN0LW5zLW1jdHdvaGIiXS9hcGkvdjEvbmFtZXNwYWNlcy90ZXN0LW5zLW1jdHdvaGIvcGVyc2lzdGVudHZvbHVtZWNsYWltcy9raWJpc2hpaS1kYXRhLWtpYmlzaGlpLWRlcGxveW1lbnQtMCokOTQxMzUyMDAtNWMwNy00NWMzLTliNDItODhlNmVhZGJmNmQ1MgQyMzg2OABCCAi+iLv+BRAAWg8KA2FwcBIIa2liaXNoaWliJgofcHYua3ViZXJuZXRlcy5pby9iaW5kLWNvbXBsZXRlZBIDeWVzYisKJHB2Lmt1YmVybmV0ZXMuaW8vYm91bmQtYnktY29udHJvbGxlchIDeWVzYkcKLXZvbHVtZS5iZXRhLmt1YmVybmV0ZXMuaW8vc3RvcmFnZS1wcm92aXNpb25lchIWY3NpLnZzcGhlcmUudm13YXJlLmNvbXIca3ViZXJuZXRlcy5pby9wdmMtcHJvdGVjdGlvbnoAigHwAwoXa3ViZS1jb250cm9sbGVyLW1hbmFnZXISBlVwZGF0ZRoCdjEiCAjwiLv+BRAAMghGaWVsZHNWMTq0AwqxA3siZjptZXRhZGF0YSI6eyJmOmFubm90YXRpb25zIjp7Ii4iOnt9LCJmOnB2Lmt1YmVybmV0ZXMuaW8vYmluZC1jb21wbGV0ZWQiOnt9LCJmOnB2Lmt1YmVybmV0ZXMuaW8vYm91bmQtYnktY29udHJvbGxlciI6e30sImY6dm9sdW1lLmJldGEua3ViZXJuZXRlcy5pby9zdG9yYWdlLXByb3Zpc2lvbmVyIjp7fX0sImY6bGFiZWxzIjp7Ii4iOnt9LCJmOmFwcCI6e319fSwiZjpzcGVjIjp7ImY6YWNjZXNzTW9kZXMiOnt9LCJmOnJlc291cmNlcyI6eyJmOnJlcXVlc3RzIjp7Ii4iOnt9LCJmOnN0b3JhZ2UiOnt9fX0sImY6c3RvcmFnZUNsYXNzTmFtZSI6e30sImY6dm9sdW1lTW9kZSI6e30sImY6dm9sdW1lTmFtZSI6e319LCJmOnN0YXR1cyI6eyJmOmFjY2Vzc01vZGVzIjp7fSwiZjpjYXBhY2l0eSI6eyIuIjp7fSwiZjpzdG9yYWdlIjp7fX0sImY6cGhhc2UiOnt9fX0ScwoNUmVhZFdyaXRlT25jZRIUEhIKB3N0b3JhZ2USBwoFMTAwTWkaKHB2Yy05NDEzNTIwMC01YzA3LTQ1YzMtOWI0Mi04OGU2ZWFkYmY2ZDUqFmtpYmlzaGlpLXN0b3JhZ2UtY2xhc3MyCkZpbGVzeXN0ZW0aKgoFQm91bmQSDVJlYWRXcml0ZU9uY2UaEgoHc3RvcmFnZRIHCgUxMDBNaQ==
  snapshotID: pvc:test-ns-mctwohb/kibishii-data-kibishii-deployment-0:aXZkOmNmNmExMWY2LThiZjMtNDk4MC1iMmZlLWU3ZjQ3OTFiYWI4MjpkYzM1ZDMyNy05MjczLTQ3ZmItYWY3OC05MWVmN2FhOTUwMTk
status:
  completionTimestamp: "2020-12-17T23:33:19Z"
  message: Download completed
  phase: Completed
  resourceHandle:
    apiGroup: ""
    kind: PersistentVolumeClaim
    name: ivd:bd95c217-6d02-4e3c-a250-24a621a1f077
```

CloneFromSnapshot CRD has some key phases for the `.status.phase` field:

* New: clone from snapshot is not completed
* InProgress: the vSphere volume snapshot is being downloaded from remote repository.
* Completed: clone from snapshot is completed
* Failed: clone from snapshot is failed
