# Changelog since v1.0.0

## v1.1.1

Date: 2021-04-16

### Breaking Changes

- Updated Astrolabe dependency to v0.3.0 ([#314](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/pull/314), [@deepakkinni](https://github.com/deepakkinni))
- Updated Makefile and go module because vmware/gvddk was renamed to vmware/virtual-disks and promoted to be an independent open-source project. ([#311](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/pull/311), [@lintongj](https://github.com/lintongj))


### Bug Fixes

- Adds support to use S3 API with self-signed certificate. ([#293](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/pull/293), [@xinyanw409](https://github.com/xinyanw409))
- Allow plugin to read non-default CSI vSphere Secrets ([#302](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/pull/302), [@deepakkinni](https://github.com/deepakkinni))
- Changed CRD parsing mechanism to not rely on SelfLink. ([#298](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/pull/298), [@dsu-igeek](https://github.com/dsu-igeek))
- Fix restored pvc does not have label issue on guest cluster. ([#316](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/pull/316), [@xinyanw409](https://github.com/xinyanw409))
- Skipped the restore of the vSphere-with-Tanzu specific resource, nsxlbmonitors.vmware.com by adding related resource names to the block list on restore ([#334](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/pull/334), [@lintongj](https://github.com/lintongj))

### Dependencies

#### Added
_Nothing has changed._

#### Changed
- github.com/vmware-tanzu/astrolabe: [d433214 → v0.3.0](https://github.com/vmware-tanzu/astrolabe/compare/d433214...v0.3.0)

#### Removed
_Nothing has changed._

## v1.1.0
### 2020-12-22

### Download
https://github.com/vmware-tanzu/velero-plugin-for-vsphere/releases/tag/v1.1.0

### Container Image
`vsphereveleroplugin/velero-plugin-for-vsphere:1.1.0`

### Release Info

This release adds the following key features.

* Full backup/restore support for applications running on Guest clusters(TKGS)
* Full backup/restore support for applications running on WCP Supervisor Cluster

### Known issues

Please refer to [known issues](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/blob/6473a7727bbfa99dd9568be108f2422bb6444eb8/docs/known_issues.md).
