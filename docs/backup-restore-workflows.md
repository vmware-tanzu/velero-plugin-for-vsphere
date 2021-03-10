# Backup Restore Workflows

## Table of Contents

1. [Guest Cluster](#guest-cluster)
2. [Vanilla or Supervisor Cluster](#vanilla-or-supervisor-cluster)

## Guest Cluster

### Backup Workflow on Guest Cluster

1. On the Guest Cluster
    1. User starts a backup using Velero CLI to backup everything in a namespace in Kubernetes, i.e., velero backup create test-backup --include-namespaces test-ns
    2. Velero backs up Kubernetes metadata to an S3 object store.
    3. Velero calls Velero Plugin for vSphere to backup a particular PVC.
    4. Velero Plugin for vSphere creates a Snapshot CR to backup the PVC.
    5. The Backup driver creates a corresponding Snapshot CR in the Supervisor Cluster.
2. On the Supervisor Cluster
    1. The Backup driver sees the newly created Snapshot CR and as a response it creates a snapshot for the specified FCD. FCD are First Class Disks on vSphere and are used to back Kubernetes storage objects, i.e. Persistent Volumes (PVs), on vSphere Storage.
    2. After the snapshot is created for the FCD, the Backup driver on the Supervisor Cluster creates an Upload CR to signal the Data Mover to upload the snapshot data.
    3. The Data Mover in the Supervisor Cluster uploads the snapshot data to S3 object store.
    4. After upload is complete, the local snapshot on FCD is deleted by the Data Mover.
3. When all PVCs are backed up, this Velero backup job is complete.

### Restore Workflow on Guest Cluster

1. On the Guest Cluster
    1. User starts a restore using Velero CLI to restore from a previous backup, i.e., velero restore create test-restore --from-backup test-backup
    2. Velero restores the metadata, e.g. namespace, etc., on Guest Cluster.
    3. Velero calls Velero Plugin for vSphere to restore a PVC.
    4. Velero Plugin for vSphere creates a CloneFromSnapshot CR.
    5. The Backup driver creates a CloneFromSnapshot CR on the Supervisor Cluster.
2. On the Supervisor Cluster
    1. The Backup driver sees the CloneFromSnapshot CR and creates a PVC on the Supervisor Cluster. This triggers the vSphere CSI driver on the Supervisor Cluster to create a new empty CNS volume/FCD on vSphere.
    2. After the FCD is created on the Supervisor Cluster, the Backup driver on the Supervisor Cluster creates a Download CR to signal the Data Mover on the Supervisor Cluster to download the snapshot data.
    3. The Data Mover downloads the snapshot data from S3 object store and overwrites the new FCD with the downloaded data.
    4. After the download is complete, the Backup driver on the Guest Cluster statically provision a PVC that points to the PVC in the Supervisor Cluster.
3. The steps will be repeated to restore every PVC that has been backed up on the Guest Cluster.
4. On the Guest Cluster, Velero restores other Kubernetes metadata.
5. When all PVCs and all Kubernetes metadata are restored, this Velero restore job is complete on the Guest Cluster.

## Vanilla or Supervisor Cluster

### Backup Workflow on Vanilla or Supervisor Cluster

1. User starts a backup using Velero CLI to backup everything in a namespace in Kubernetes, i.e., velero backup create test-backup --include-namespaces test-ns
2. Velero backs up Kubernetes metadata to an S3 object store.
3. Velero calls Velero Plugin for vSphere to backup a particular PVC.
4. Velero Plugin for vSphere creates a Snapshot CR to backup the PVC.
5. The Backup driver sees the newly created Snapshot CR and as a response it creates a snapshot for the specified FCD. FCD are First Class Disks on vSphere and are used to back Kubernetes storage objects, i.e. Persistent Volumes (PVs), on vSphere Storage.
6. After the snapshot is created for the FCD, the Backup driver creates an Upload CR to signal the Data Mover to upload the snapshot data.
7. The Data Mover uploads the snapshot data to S3 object store.
8. After upload is complete, the local snapshot on FCD is deleted by the Data Mover.
9. Steps 3-8 will be repeated for every PVC that is backed up.
10. When all PVCs are backed up, this Velero backup job is complete.

### Restore Workflow on Vanilla or Supervisor Cluster

1. User starts a restore using Velero CLI to restore from a previous backup, i.e., velero restore create test-restore --from-backup test-backup
2. Velero restores the metadata, e.g. namespace, etc.
3. Velero calls Velero Plugin for vSphere to restore a PVC.
4. Velero Plugin for vSphere creates a CloneFromSnapshot CR.
5. The Backup driver sees the CloneFromSnapshot CR and creates a PVC on the cluster. This triggers the vSphere CSI driver to create a new empty CNS volume/FCD on vSphere.
6. After the FCD is created, the Backup driver creates a Download CR to signal the Data Mover to download the snapshot data.
7. The Data Mover downloads the snapshot data from S3 object store and overwrites the new FCD with the downloaded data.
8. Steps 3-7 will be repeated to restore every PVC that has been backed up.
9. Velero restores other Kubernetes metadata.
10. When all PVCs and all Kubernetes metadata are restored, this Velero restore job is complete.
