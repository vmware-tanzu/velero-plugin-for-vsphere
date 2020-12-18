# Backward Compatibility

Backup taken by Velero vSphere plugin with version lower or equal to 1.0.2 can be restored by Velero vSphere plugin version 1.1.0
only on a Vanilla Setup.

Prior to attempting restore or delete of backups created using plugin version <= 1.0.2, ensure that a vSphere snapshot
location exists. If not, create one.

```bash
velero snapshot-location create <snapshot location name> --provider velero.io/vsphere
```

If there are multiple [BackupStorageLocations](https://velero.io/docs/main/api-types/backupstoragelocation/) defined during
restore time, it is essential that the BackupStorageLocation used for the backups created using plugin version <= 1.0.2 is
is made the "default" BackupStorageLocation.

After restore/delete of backups created using plugin version <= 1.0.2, the vSphere snapshot location can be safely deleted.
The vSphere snapshot location is only necessary when attempting to restore or delete of backups created using plugin
version <= 1.0.2.

Attempts to delete backups created using plugin version <= 1.0.2 without a vSphere Snapshot location defined will lead
to dangling snapshot data in the remote repository. This may need to be removed manually.

Restore of a Backup created in Vanilla setup to a Guest Cluster is not supported.

Restore of a Backup created in Supervisor Cluster to a Guest Cluster is not supported.

Restore of a Backup created in Guest Cluster to a Vanilla setup is not supported.

Restore of a Backup created in Guest Cluster to a Supervisor Cluster is not supported.