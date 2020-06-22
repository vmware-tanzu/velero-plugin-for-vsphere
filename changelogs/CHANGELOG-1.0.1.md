## v1.0.1
### 2020-06-16

### Download
https://github.com/vmware-tanzu/velero-plugin-for-vsphere/releases/tag/v1.0.1

### Container Image
`vsphereveleroplugin/velero-plugin-for-vsphere:1.0.1`

### Release Info

This release fixes problems related to Kubernetes installed using Essentials PKS.

### Known issues

The restore might place the restored PVs in a vSphere datastore which is imcompatible with the StorageClass
specified in the restored PVC, as we don't rely on the StorageClass for the PV placement on restore in the release v1.0.0.

### All changes

Code cleanup: Removed unused files and cleaned up unused code snippets

Add unit tests for velero plug-in for vsphere.
Picked up the fix, which enables to find node VMs in sub-folders of datacenter inventory in the restore path, from the Astrolabe at 1f6b7a39655ff34335899bb5b08310a5f6e2c872. 

Add uninstall steps to delete data manager and related resources.
Fix to get vSphere credentials for TKG cluster.
Fix issue that cannot backup PVs which are not attached to volumes, support backup independent PVs.
This change fix hardcoding for data manager image repository, add 'make release' command to push qualified images to official repository.
