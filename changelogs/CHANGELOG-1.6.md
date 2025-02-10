# Changelog since v1.5.4


## v1.6.0

Date: 2025-02-10

### Changes

- Block StorageQuota related CRDs from backup ([#593](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/pull/593), [@deepakkinni](https://github.com/deepakkinni))
- Caymanize velero vSphere plugin images ([#605](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/pull/605), [@nikhilbarge](https://github.com/nikhilbarge))
- Documentation refresh ([#600](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/pull/600), [@nikhilbarge](https://github.com/nikhilbarge))
- Fixed a problem in backup in the VDS setup on Supervisor. ([#578](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/pull/578), [@varunsrinivasan2](https://github.com/varunsrinivasan2))
- For Supervisor's using NSX Operator with VPC Networking, Velero backups will omit all NSX Operator v1alpha1 VPC APIs. vSphere Pods, Services, and any other resource's networking-related properties will need to be re-created by NSX Operator upon restore. ([#582](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/pull/582), [@ihgann](https://github.com/ihgann))
- Photon 5 Upgrade ([#594](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/pull/594), [@deepakkinni](https://github.com/deepakkinni))
- This change updates the blocked NSX Operator CRDs to point to their new API Group, `crd.nsx.vmware.com`, and reconciles all new CRDs being pushed by NSX Operator. ([#591](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/pull/591), [@ihgann](https://github.com/ihgann))
- Updated Vanilla installation document. ([#569](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/pull/569), [@varunsrinivasan2](https://github.com/varunsrinivasan2))

## Dependencies

### Added
_Nothing has changed._

### Changed
- github.com/agiledragon/gomonkey: [v2.0.1+incompatible → v2.0.2+incompatible](https://github.com/agiledragon/gomonkey/compare/v2.0.1...v2.0.2)
- github.com/stretchr/objx: [v0.5.0 → v0.5.2](https://github.com/stretchr/objx/compare/v0.5.0...v0.5.2)
- github.com/stretchr/testify: [v1.8.1 → v1.10.0](https://github.com/stretchr/testify/compare/v1.8.1...v1.10.0)
- golang.org/x/crypto: v0.17.0 → v0.21.0
- golang.org/x/net: v0.17.0 → v0.23.0
- golang.org/x/sys: v0.15.0 → v0.18.0
- golang.org/x/term: v0.15.0 → v0.18.0
- google.golang.org/protobuf: v1.30.0 → v1.33.0

### Removed
_Nothing has changed._
