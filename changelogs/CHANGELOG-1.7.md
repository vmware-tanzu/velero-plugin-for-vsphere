# Changelog since v1.6.0

## v1.7.0

Date: 2026-06-18

### Changes

- Add Carvel `packages.data.packaging.carvel.dev` and `packagemetadatas.data.packaging.carvel.dev` to the list of CRDs blocked from backup. ([#628](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/pull/628), [@ckhare](https://github.com/ckhare))
- Updated dependencies to support Velero v1.18.1 and Kubernetes v0.36.0. ([#630](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/pull/630), [@xing-yang](https://github.com/xing-yang))

## Dependencies

### Added
_Nothing has changed._

### Changed
- github.com/golang-jwt/jwt/v4: [v4.4.1 → v4.5.2](https://github.com/golang-jwt/jwt/compare/v4.4.1...v4.5.2)
- github.com/google/uuid: [v1.3.0 → v1.6.0](https://github.com/google/uuid/compare/v1.3.0...v1.6.0)
- github.com/sirupsen/logrus: [v1.8.1 → v1.9.4](https://github.com/sirupsen/logrus/compare/v1.8.1...v1.9.4)
- github.com/spf13/cobra: [v1.4.0 → v1.10.2](https://github.com/spf13/cobra/compare/v1.4.0...v1.10.2)
- github.com/spf13/pflag: [v1.0.5 → v1.0.10](https://github.com/spf13/pflag/compare/v1.0.5...v1.0.10)
- github.com/stretchr/testify: [v1.10.0 → v1.11.1](https://github.com/stretchr/testify/compare/v1.10.0...v1.11.1)
- github.com/vmware-tanzu/velero: [v1.10.2 → v1.18.1](https://github.com/vmware-tanzu/velero/compare/v1.10.2...v1.18.1)
- golang.org/x/crypto: v0.21.0 → v0.50.0
- golang.org/x/net: v0.23.0 → v0.53.0
- golang.org/x/oauth2: v0.7.0 → v0.36.0
- golang.org/x/sys: v0.18.0 → v0.43.0
- golang.org/x/term: v0.18.0 → v0.42.0
- google.golang.org/grpc: v1.56.3 → v1.80.0
- google.golang.org/protobuf: v1.33.0 → v1.36.12-0.20260120151049-f2248ac996af
- k8s.io/api: [v0.24.2 → v0.36.0](https://github.com/kubernetes/api/compare/v0.24.2...v0.36.0)
- k8s.io/apiextensions-apiserver: [v0.24.2 → v0.36.0](https://github.com/kubernetes/apiextensions-apiserver/compare/v0.24.2...v0.36.0)
- k8s.io/apimachinery: [v0.24.2 → v0.36.0](https://github.com/kubernetes/apimachinery/compare/v0.24.2...v0.36.0)
- k8s.io/client-go: [v0.24.2 → v0.36.0](https://github.com/kubernetes/client-go/compare/v0.24.2...v0.36.0)
- k8s.io/utils: v0.0.0-20220210201930-3a6ce19ff2f9 → v0.0.0-20260210185600-b8788abfbbc2
- sigs.k8s.io/controller-runtime: [v0.12.2 → v0.24.1](https://github.com/kubernetes-sigs/controller-runtime/compare/v0.12.2...v0.24.1)

### Removed
_Nothing has changed._
