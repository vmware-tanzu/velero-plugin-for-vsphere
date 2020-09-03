## v1.0.2
### 2020-09-03

### Download
https://github.com/vmware-tanzu/velero-plugin-for-vsphere/releases/tag/v1.0.2

### Container Image
`vsphereveleroplugin/velero-plugin-for-vsphere:1.0.2`

### Release Info

This release fixes a data loss issue where the last several megabytes of a snapshotted volume would not be uploaded to S3.
Also fixes issues related to different site configurations
All container base images are updated to Ubuntu Focal

### Known issues

The restore might place the restored PVs in a vSphere datastore which is imcompatible with the StorageClass
specified in the restored PVC, as we don't rely on the StorageClass for the PV placement on restore in the release v1.0.0.

### All changes

Updated to astrolabe v0.1.2-0.20200831235727-748c63d0fa96 for missing block fix

Applied fix for special characters in password
Added unit test for special characters in password

Fix failed pull data manager image issue
    
Change plugin dockerfile and datamgr dockerfile to use ubuntu focal
    
