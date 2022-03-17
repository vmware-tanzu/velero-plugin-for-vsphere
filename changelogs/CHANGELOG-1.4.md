# Changelog since v1.3.0

## v1.4.0

Date: 2022-03-16

### Changes

- Add actions that sync images to GCR on release ([#450](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/pull/450), [@blackpiglet](https://github.com/blackpiglet))
- Bump up astrolabe version to pick up csi migrated volume support ([#442](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/pull/442), [@liuy1vmware](https://github.com/liuy1vmware))
- Bump up astrolabe version to v0.5.0 ([#454](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/pull/454), [@lintongj](https://github.com/lintongj))
- Process DeleteSnapshot CRs without explicit Phase set ([#441](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/pull/441), [@deepakkinni](https://github.com/deepakkinni))
- Support creating snapshot for migrated CSI volume ([#443](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/pull/443), [@liuy1vmware](https://github.com/liuy1vmware))
- Upgrade golang version to 1.17 ([#448](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/pull/448), [@xinyanw409](https://github.com/xinyanw409))

## Dependencies

### Added
- github.com/xdg-go/pbkdf2: [v1.0.0](https://github.com/xdg-go/pbkdf2/tree/v1.0.0)
- github.com/xdg-go/scram: [v1.0.2](https://github.com/xdg-go/scram/tree/v1.0.2)
- github.com/xdg-go/stringprep: [v1.0.2](https://github.com/xdg-go/stringprep/tree/v1.0.2)
- github.com/youmark/pkcs8: [1be2e3e](https://github.com/youmark/pkcs8/tree/1be2e3e)

### Changed
- github.com/asaskevich/govalidator: [475eaeb → 7a23bdc](https://github.com/asaskevich/govalidator/compare/475eaeb...7a23bdc)
- github.com/go-openapi/analysis: [v0.19.10 → v0.20.1](https://github.com/go-openapi/analysis/compare/v0.19.10...v0.20.1)
- github.com/go-openapi/errors: [v0.19.4 → v0.20.1](https://github.com/go-openapi/errors/compare/v0.19.4...v0.20.1)
- github.com/go-openapi/jsonreference: [v0.19.5 → v0.19.6](https://github.com/go-openapi/jsonreference/compare/v0.19.5...v0.19.6)
- github.com/go-openapi/loads: [v0.19.5 → v0.21.0](https://github.com/go-openapi/loads/compare/v0.19.5...v0.21.0)
- github.com/go-openapi/runtime: [v0.19.12 → v0.21.0](https://github.com/go-openapi/runtime/compare/v0.19.12...v0.21.0)
- github.com/go-openapi/spec: [v0.19.7 → v0.20.4](https://github.com/go-openapi/spec/compare/v0.19.7...v0.20.4)
- github.com/go-openapi/strfmt: [v0.19.5 → v0.21.1](https://github.com/go-openapi/strfmt/compare/v0.19.5...v0.21.1)
- github.com/go-openapi/swag: [v0.19.14 → v0.19.15](https://github.com/go-openapi/swag/compare/v0.19.14...v0.19.15)
- github.com/go-openapi/validate: [v0.19.7 → v0.20.3](https://github.com/go-openapi/validate/compare/v0.19.7...v0.20.3)
- github.com/klauspost/compress: [v1.9.5 → v1.13.6](https://github.com/klauspost/compress/compare/v1.9.5...v1.13.6)
- github.com/mitchellh/mapstructure: [v1.1.2 → v1.4.1](https://github.com/mitchellh/mapstructure/compare/v1.1.2...v1.4.1)
- github.com/opentracing/opentracing-go: [v1.1.0 → v1.2.0](https://github.com/opentracing/opentracing-go/compare/v1.1.0...v1.2.0)
- github.com/pelletier/go-toml: [v1.6.0 → v1.7.0](https://github.com/pelletier/go-toml/compare/v1.6.0...v1.7.0)
- github.com/vmware-tanzu/astrolabe: [12eb18c → v0.5.0](https://github.com/vmware-tanzu/astrolabe/compare/12eb18c...v0.5.0)
- go.mongodb.org/mongo-driver: v1.3.1 → v1.7.5
- golang.org/x/text: v0.3.6 → v0.3.7
- sigs.k8s.io/structured-merge-diff/v3: 67a7b8c → v3.0.0

### Removed
_Nothing has changed._
