# Change log since v1.2.0

## v1.2.1

Date: 2021-08-03

### Changes

- Cherry-pick of ([#373](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/pull/373). Do not retrieve supervisor BackupRepository when it is unspecified. ([#375](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/pull/375), [@deepakkinni](https://github.com/deepakkinni))

### Dependencies

#### Added
_Nothing has changed._

#### Changed
_Nothing has changed._

#### Removed
_Nothing has changed._

# Changelog since v1.1.0

## v1.2.0

Date: 2021-07-28

### Changes

- Added the check of restic annotation in vsphere plugin to skip pvc snapshot for restic annotated volumes. ([#349](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/pull/349), [@lintongj](https://github.com/lintongj))
- Added vSphere CSI Driver 2.3 support in plugin by updating the way to determine cluster flavor and determine the secret to watch. ([#348](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/pull/348), [@deepakkinni](https://github.com/deepakkinni))
- Get rid of the dependency on the retrieval of the in cluster k8s config and support running backup-driver and data-manager both in-cluster and out-of-cluster. ([#346](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/pull/346), [@lintongj](https://github.com/lintongj))
- Make vddk log level configurable. ([#354](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/pull/354), [@xinyanw409](https://github.com/xinyanw409))
- Skips PVC restore if it already exists. ([#341](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/pull/341), [@xing-yang](https://github.com/xing-yang))
- Upgraded CRD API version from v1beta1 to v1 ([#368](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/pull/368), [@lintongj](https://github.com/lintongj))
- Upgraded VDDK library in dependency from vSphere 7.0 to 7.0U2. ([#352](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/pull/352), [@xinyanw409](https://github.com/xinyanw409))

### Dependencies

#### Added
_Nothing has changed._

#### Changed
- github.com/vmware-tanzu/astrolabe: [v0.3.0 → v0.4.0](https://github.com/vmware-tanzu/astrolabe/compare/v0.3.0...v0.4.0)
- github.com/vmware/virtual-disks: [v0.0.2 → v0.0.4](https://github.com/vmware/virtual-disks/compare/v0.0.2...v0.0.4)

#### Removed
_Nothing has changed._
