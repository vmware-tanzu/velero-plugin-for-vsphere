# Changelog since v1.3.0

## v1.3.1

Date: 2021-12-21

### Changes

- Cherry-pick [#422](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/pull/422): Add support for velero image with sha tag. ([#431](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/pull/431), [@deepakkinni](https://github.com/deepakkinni))
- Cherry-pick [#434](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/pull/434): Retry if there are errors retrieving Download CR. ([#435](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/pull/435), [@deepakkinni](https://github.com/deepakkinni))

### Dependencies

#### Added
_Nothing has changed._

#### Changed
_Nothing has changed._

#### Removed
_Nothing has changed._

# Changelog since v1.2.1

## v1.3.0

Date: 2021-11-10

### Changes

- Enable decouple-vsphere-csi-driver by default. ([#411](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/pull/411), [@deepakkinni](https://github.com/deepakkinni))
- Fixed the issue with parsing NetworkAttachmentDefinition resource and ensure it is blocked in backup ([#392](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/pull/392), [@lintongj](https://github.com/lintongj))
- Ignore older vcenter logout errors when reloading configuration with a new username password. ([#396](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/pull/396), [@deepakkinni](https://github.com/deepakkinni))
- Insecure and data-center params need not be specified when creating velero-vsphere-config-secret that contain vc credentials. ([#397](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/pull/397), [@deepakkinni](https://github.com/deepakkinni))
- Moved the vSphere specific ivd Protected Entity from Astrolabe into Velero Plugin for vSphere, no functionality
  changes in the IVD PE otherwise. Added vsphere-astrolabe CLI tool that can interact with IVDs ([#386](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/pull/386), [@dsu-igeek](https://github.com/dsu-igeek))
- This change requires a `velero-vsphere-plugin-config` ConfigMap that contains the cluster type information to be created prior to plugin installation for all cluster favors. This change also requires a `velero-vsphere-config-secret` Secret to be created in the same namespace as velero which contains the VC credentials on Vanilla Cluster setups. ([#391](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/pull/391), [@deepakkinni](https://github.com/deepakkinni))
- Update to use distroless base image. ([#383](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/pull/383), [@xinyanw409](https://github.com/xinyanw409))
- Update to use golang 1.16. ([#388](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/pull/388), [@xinyanw409](https://github.com/xinyanw409))
- Updated the default image registry to the official registry for the fall-back mechanism ([#380](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/pull/380), [@lintongj](https://github.com/lintongj))

### Dependencies

#### Added
- cloud.google.com/go/firestore: v1.1.0
- cloud.google.com/go/storage: v1.6.0
- dmitri.shuralyov.com/gpu/mtl: 666a987
- github.com/Azure/go-autorest: [v14.2.0+incompatible](https://github.com/Azure/go-autorest/tree/v14.2.0)
- github.com/antihax/optional: [v1.0.0](https://github.com/antihax/optional/tree/v1.0.0)
- github.com/armon/go-metrics: [f0300d1](https://github.com/armon/go-metrics/tree/f0300d1)
- github.com/armon/go-radix: [7fddfc3](https://github.com/armon/go-radix/tree/7fddfc3)
- github.com/benbjohnson/clock: [v1.0.3](https://github.com/benbjohnson/clock/tree/v1.0.3)
- github.com/bketelsen/crypt: [5cbc8cc](https://github.com/bketelsen/crypt/tree/5cbc8cc)
- github.com/certifi/gocertifi: [2c3bb06](https://github.com/certifi/gocertifi/tree/2c3bb06)
- github.com/chzyer/logex: [v1.1.10](https://github.com/chzyer/logex/tree/v1.1.10)
- github.com/chzyer/readline: [2972be2](https://github.com/chzyer/readline/tree/2972be2)
- github.com/chzyer/test: [a1ea475](https://github.com/chzyer/test/tree/a1ea475)
- github.com/cockroachdb/errors: [v1.2.4](https://github.com/cockroachdb/errors/tree/v1.2.4)
- github.com/cockroachdb/logtags: [eb05cc2](https://github.com/cockroachdb/logtags/tree/eb05cc2)
- github.com/coreos/go-systemd/v22: [v22.3.2](https://github.com/coreos/go-systemd/v22/tree/v22.3.2)
- github.com/felixge/httpsnoop: [v1.0.1](https://github.com/felixge/httpsnoop/tree/v1.0.1)
- github.com/form3tech-oss/jwt-go: [v3.2.3+incompatible](https://github.com/form3tech-oss/jwt-go/tree/v3.2.3)
- github.com/fvbommel/sortorder: [v1.0.1](https://github.com/fvbommel/sortorder/tree/v1.0.1)
- github.com/getsentry/raven-go: [v0.2.0](https://github.com/getsentry/raven-go/tree/v0.2.0)
- github.com/go-errors/errors: [v1.0.1](https://github.com/go-errors/errors/tree/v1.0.1)
- github.com/go-gl/glfw/v3.3/glfw: [6f7a984](https://github.com/go-gl/glfw/v3.3/glfw/tree/6f7a984)
- github.com/go-gl/glfw: [e6da0ac](https://github.com/go-gl/glfw/tree/e6da0ac)
- github.com/go-kit/log: [v0.1.0](https://github.com/go-kit/log/tree/v0.1.0)
- github.com/godbus/dbus/v5: [v5.0.4](https://github.com/godbus/dbus/v5/tree/v5.0.4)
- github.com/google/shlex: [e7afc7f](https://github.com/google/shlex/tree/e7afc7f)
- github.com/hashicorp/consul/api: [v1.1.0](https://github.com/hashicorp/consul/api/tree/v1.1.0)
- github.com/hashicorp/consul/sdk: [v0.1.1](https://github.com/hashicorp/consul/sdk/tree/v0.1.1)
- github.com/hashicorp/errwrap: [v1.0.0](https://github.com/hashicorp/errwrap/tree/v1.0.0)
- github.com/hashicorp/go-cleanhttp: [v0.5.1](https://github.com/hashicorp/go-cleanhttp/tree/v0.5.1)
- github.com/hashicorp/go-immutable-radix: [v1.0.0](https://github.com/hashicorp/go-immutable-radix/tree/v1.0.0)
- github.com/hashicorp/go-msgpack: [v0.5.3](https://github.com/hashicorp/go-msgpack/tree/v0.5.3)
- github.com/hashicorp/go-multierror: [v1.0.0](https://github.com/hashicorp/go-multierror/tree/v1.0.0)
- github.com/hashicorp/go-rootcerts: [v1.0.0](https://github.com/hashicorp/go-rootcerts/tree/v1.0.0)
- github.com/hashicorp/go-sockaddr: [v1.0.0](https://github.com/hashicorp/go-sockaddr/tree/v1.0.0)
- github.com/hashicorp/go-uuid: [v1.0.1](https://github.com/hashicorp/go-uuid/tree/v1.0.1)
- github.com/hashicorp/go.net: [v0.0.1](https://github.com/hashicorp/go.net/tree/v0.0.1)
- github.com/hashicorp/logutils: [v1.0.0](https://github.com/hashicorp/logutils/tree/v1.0.0)
- github.com/hashicorp/mdns: [v1.0.0](https://github.com/hashicorp/mdns/tree/v1.0.0)
- github.com/hashicorp/memberlist: [v0.1.3](https://github.com/hashicorp/memberlist/tree/v0.1.3)
- github.com/hashicorp/serf: [v0.8.2](https://github.com/hashicorp/serf/tree/v0.8.2)
- github.com/ianlancetaylor/demangle: [5e5cf60](https://github.com/ianlancetaylor/demangle/tree/5e5cf60)
- github.com/jmespath/go-jmespath/internal/testify: [v1.5.1](https://github.com/jmespath/go-jmespath/internal/testify/tree/v1.5.1)
- github.com/josharian/intern: [v1.0.0](https://github.com/josharian/intern/tree/v1.0.0)
- github.com/jpillora/backoff: [v1.0.0](https://github.com/jpillora/backoff/tree/v1.0.0)
- github.com/mitchellh/cli: [v1.0.0](https://github.com/mitchellh/cli/tree/v1.0.0)
- github.com/mitchellh/gox: [v0.4.0](https://github.com/mitchellh/gox/tree/v0.4.0)
- github.com/mitchellh/iochan: [v1.0.0](https://github.com/mitchellh/iochan/tree/v1.0.0)
- github.com/moby/spdystream: [v0.2.0](https://github.com/moby/spdystream/tree/v0.2.0)
- github.com/moby/term: [9d4ed18](https://github.com/moby/term/tree/9d4ed18)
- github.com/monochromegane/go-gitignore: [205db1a](https://github.com/monochromegane/go-gitignore/tree/205db1a)
- github.com/niemeyer/pretty: [a10e7ca](https://github.com/niemeyer/pretty/tree/a10e7ca)
- github.com/opentracing/opentracing-go: [v1.1.0](https://github.com/opentracing/opentracing-go/tree/v1.1.0)
- github.com/pascaldekloe/goe: [57f6aae](https://github.com/pascaldekloe/goe/tree/57f6aae)
- github.com/posener/complete: [v1.1.1](https://github.com/posener/complete/tree/v1.1.1)
- github.com/ryanuber/columnize: [9b3edd6](https://github.com/ryanuber/columnize/tree/9b3edd6)
- github.com/sean-/seed: [e2103e2](https://github.com/sean-/seed/tree/e2103e2)
- github.com/stoewer/go-strcase: [v1.2.0](https://github.com/stoewer/go-strcase/tree/v1.2.0)
- github.com/xlab/treeprint: [a009c39](https://github.com/xlab/treeprint/tree/a009c39)
- github.com/yuin/goldmark: [v1.3.5](https://github.com/yuin/goldmark/tree/v1.3.5)
- go.etcd.io/etcd/api/v3: v3.5.0
- go.etcd.io/etcd/client/pkg/v3: v3.5.0
- go.etcd.io/etcd/client/v2: v2.305.0
- go.etcd.io/etcd/client/v3: v3.5.0
- go.etcd.io/etcd/pkg/v3: v3.5.0
- go.etcd.io/etcd/raft/v3: v3.5.0
- go.etcd.io/etcd/server/v3: v3.5.0
- go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc: v0.20.0
- go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp: v0.20.0
- go.opentelemetry.io/contrib: v0.20.0
- go.opentelemetry.io/otel/exporters/otlp: v0.20.0
- go.opentelemetry.io/otel/metric: v0.20.0
- go.opentelemetry.io/otel/oteltest: v0.20.0
- go.opentelemetry.io/otel/sdk/export/metric: v0.20.0
- go.opentelemetry.io/otel/sdk/metric: v0.20.0
- go.opentelemetry.io/otel/sdk: v0.20.0
- go.opentelemetry.io/otel/trace: v0.20.0
- go.opentelemetry.io/otel: v0.20.0
- go.opentelemetry.io/proto/otlp: v0.7.0
- go.starlark.net: 8dd3e2e
- go.uber.org/goleak: v1.1.10
- golang.org/x/term: 6a3ed07
- gotest.tools/v3: v3.0.3
- k8s.io/component-helpers: v0.22.0
- k8s.io/controller-manager: v0.22.0
- k8s.io/mount-utils: v0.22.0
- sigs.k8s.io/kustomize/api: v0.8.11
- sigs.k8s.io/kustomize/cmd/config: v0.9.13
- sigs.k8s.io/kustomize/kustomize/v4: v4.2.0
- sigs.k8s.io/kustomize/kyaml: v0.11.0
- sigs.k8s.io/structured-merge-diff/v4: v4.1.2

#### Changed
- cloud.google.com/go/bigquery: v1.0.1 → v1.4.0
- cloud.google.com/go/datastore: v1.0.0 → v1.1.0
- cloud.google.com/go/pubsub: v1.0.1 → v1.2.0
- cloud.google.com/go: v0.46.2 → v0.54.0
- github.com/Azure/azure-sdk-for-go: [v42.0.0+incompatible → v55.0.0+incompatible](https://github.com/Azure/azure-sdk-for-go/compare/v42.0.0...v55.0.0)
- github.com/Azure/go-ansiterm: [d6e3b33 → d185dfc](https://github.com/Azure/go-ansiterm/compare/d6e3b33...d185dfc)
- github.com/Azure/go-autorest/autorest/adal: [v0.8.2 → v0.9.13](https://github.com/Azure/go-autorest/autorest/adal/compare/v0.8.2...v0.9.13)
- github.com/Azure/go-autorest/autorest/date: [v0.2.0 → v0.3.0](https://github.com/Azure/go-autorest/autorest/date/compare/v0.2.0...v0.3.0)
- github.com/Azure/go-autorest/autorest/mocks: [v0.3.0 → v0.4.1](https://github.com/Azure/go-autorest/autorest/mocks/compare/v0.3.0...v0.4.1)
- github.com/Azure/go-autorest/autorest/to: [v0.3.0 → v0.4.0](https://github.com/Azure/go-autorest/autorest/to/compare/v0.3.0...v0.4.0)
- github.com/Azure/go-autorest/autorest: [v0.9.6 → v0.11.18](https://github.com/Azure/go-autorest/autorest/compare/v0.9.6...v0.11.18)
- github.com/Azure/go-autorest/logger: [v0.1.0 → v0.2.1](https://github.com/Azure/go-autorest/logger/compare/v0.1.0...v0.2.1)
- github.com/Azure/go-autorest/tracing: [v0.5.0 → v0.6.0](https://github.com/Azure/go-autorest/tracing/compare/v0.5.0...v0.6.0)
- github.com/GoogleCloudPlatform/k8s-cloud-provider: [27a4ced → 7901bc8](https://github.com/GoogleCloudPlatform/k8s-cloud-provider/compare/27a4ced...7901bc8)
- github.com/NYTimes/gziphandler: [56545f4 → v1.1.1](https://github.com/NYTimes/gziphandler/compare/56545f4...v1.1.1)
- github.com/alecthomas/units: [c3de453 → f65c72e](https://github.com/alecthomas/units/compare/c3de453...f65c72e)
- github.com/aws/aws-sdk-go: [v1.29.19 → v1.38.49](https://github.com/aws/aws-sdk-go/compare/v1.29.19...v1.38.49)
- github.com/cncf/udpa/go: [269d4d4 → 5459f2c](https://github.com/cncf/udpa/go/compare/269d4d4...5459f2c)
- github.com/cockroachdb/datadriven: [80d97fb → bf6692d](https://github.com/cockroachdb/datadriven/compare/80d97fb...bf6692d)
- github.com/coreos/etcd: [v3.3.10+incompatible → v3.3.13+incompatible](https://github.com/coreos/etcd/compare/v3.3.10...v3.3.13)
- github.com/creack/pty: [v1.1.9 → v1.1.11](https://github.com/creack/pty/compare/v1.1.9...v1.1.11)
- github.com/envoyproxy/go-control-plane: [v0.9.4 → 668b12f](https://github.com/envoyproxy/go-control-plane/compare/v0.9.4...668b12f)
- github.com/evanphx/json-patch: [v4.5.0+incompatible → v4.11.0+incompatible](https://github.com/evanphx/json-patch/compare/v4.5.0...v4.11.0)
- github.com/go-logfmt/logfmt: [v0.4.0 → v0.5.0](https://github.com/go-logfmt/logfmt/compare/v0.4.0...v0.5.0)
- github.com/go-logr/logr: [v0.1.0 → v0.4.0](https://github.com/go-logr/logr/compare/v0.1.0...v0.4.0)
- github.com/go-openapi/jsonpointer: [v0.19.3 → v0.19.5](https://github.com/go-openapi/jsonpointer/compare/v0.19.3...v0.19.5)
- github.com/go-openapi/jsonreference: [v0.19.3 → v0.19.5](https://github.com/go-openapi/jsonreference/compare/v0.19.3...v0.19.5)
- github.com/go-openapi/swag: [v0.19.8 → v0.19.14](https://github.com/go-openapi/swag/compare/v0.19.8...v0.19.14)
- github.com/gofrs/uuid: [v3.2.0+incompatible → v4.0.0+incompatible](https://github.com/gofrs/uuid/compare/v3.2.0...v4.0.0)
- github.com/gogo/protobuf: [v1.3.1 → v1.3.2](https://github.com/gogo/protobuf/compare/v1.3.1...v1.3.2)
- github.com/golang/groupcache: [5b532d6 → 41bb18b](https://github.com/golang/groupcache/compare/5b532d6...41bb18b)
- github.com/golang/mock: [v1.4.3 → v1.4.4](https://github.com/golang/mock/compare/v1.4.3...v1.4.4)
- github.com/golang/protobuf: [v1.4.2 → v1.5.2](https://github.com/golang/protobuf/compare/v1.4.2...v1.5.2)
- github.com/google/btree: [v1.0.0 → v1.0.1](https://github.com/google/btree/compare/v1.0.0...v1.0.1)
- github.com/google/go-cmp: [v0.5.0 → v0.5.5](https://github.com/google/go-cmp/compare/v0.5.0...v0.5.5)
- github.com/google/pprof: [54271f7 → 1ebb73c](https://github.com/google/pprof/compare/54271f7...1ebb73c)
- github.com/google/uuid: [v1.1.1 → v1.1.2](https://github.com/google/uuid/compare/v1.1.1...v1.1.2)
- github.com/googleapis/gnostic: [v0.3.1 → v0.5.5](https://github.com/googleapis/gnostic/compare/v0.3.1...v0.5.5)
- github.com/gorilla/websocket: [v1.4.0 → v1.4.2](https://github.com/gorilla/websocket/compare/v1.4.0...v1.4.2)
- github.com/grpc-ecosystem/go-grpc-middleware: [f849b54 → v1.3.0](https://github.com/grpc-ecosystem/go-grpc-middleware/compare/f849b54...v1.3.0)
- github.com/grpc-ecosystem/grpc-gateway: [v1.9.5 → v1.16.0](https://github.com/grpc-ecosystem/grpc-gateway/compare/v1.9.5...v1.16.0)
- github.com/jmespath/go-jmespath: [c2b33e8 → v0.4.0](https://github.com/jmespath/go-jmespath/compare/c2b33e8...v0.4.0)
- github.com/jonboulle/clockwork: [v0.1.0 → v0.2.2](https://github.com/jonboulle/clockwork/compare/v0.1.0...v0.2.2)
- github.com/json-iterator/go: [v1.1.10 → v1.1.11](https://github.com/json-iterator/go/compare/v1.1.10...v1.1.11)
- github.com/jstemmer/go-junit-report: [af01ea7 → v0.9.1](https://github.com/jstemmer/go-junit-report/compare/af01ea7...v0.9.1)
- github.com/julienschmidt/httprouter: [v1.2.0 → v1.3.0](https://github.com/julienschmidt/httprouter/compare/v1.2.0...v1.3.0)
- github.com/kisielk/errcheck: [v1.2.0 → v1.5.0](https://github.com/kisielk/errcheck/compare/v1.2.0...v1.5.0)
- github.com/konsorten/go-windows-terminal-sequences: [v1.0.2 → v1.0.3](https://github.com/konsorten/go-windows-terminal-sequences/compare/v1.0.2...v1.0.3)
- github.com/mailru/easyjson: [v0.7.1 → v0.7.6](https://github.com/mailru/easyjson/compare/v0.7.1...v0.7.6)
- github.com/mattn/go-runewidth: [v0.0.2 → v0.0.7](https://github.com/mattn/go-runewidth/compare/v0.0.2...v0.0.7)
- github.com/matttproud/golang_protobuf_extensions: [v1.0.1 → c182aff](https://github.com/matttproud/golang_protobuf_extensions/compare/v1.0.1...c182aff)
- github.com/mwitkow/go-conntrack: [cc309e4 → 2f06839](https://github.com/mwitkow/go-conntrack/compare/cc309e4...2f06839)
- github.com/olekukonko/tablewriter: [a0225b3 → v0.0.4](https://github.com/olekukonko/tablewriter/compare/a0225b3...v0.0.4)
- github.com/onsi/ginkgo: [v1.13.0 → v1.14.0](https://github.com/onsi/ginkgo/compare/v1.13.0...v1.14.0)
- github.com/prometheus/client_golang: [v1.5.1 → v1.11.0](https://github.com/prometheus/client_golang/compare/v1.5.1...v1.11.0)
- github.com/prometheus/common: [v0.9.1 → v0.26.0](https://github.com/prometheus/common/compare/v0.9.1...v0.26.0)
- github.com/prometheus/procfs: [v0.0.11 → v0.6.0](https://github.com/prometheus/procfs/compare/v0.0.11...v0.6.0)
- github.com/rogpeppe/fastuuid: [6724a57 → v1.2.0](https://github.com/rogpeppe/fastuuid/compare/6724a57...v1.2.0)
- github.com/rubiojr/go-vhd: [0bfd3b3 → 02e2102](https://github.com/rubiojr/go-vhd/compare/0bfd3b3...02e2102)
- github.com/sergi/go-diff: [v1.0.0 → v1.1.0](https://github.com/sergi/go-diff/compare/v1.0.0...v1.1.0)
- github.com/sirupsen/logrus: [v1.4.2 → v1.8.1](https://github.com/sirupsen/logrus/compare/v1.4.2...v1.8.1)
- github.com/soheilhy/cmux: [v0.1.4 → v0.1.5](https://github.com/soheilhy/cmux/compare/v0.1.4...v0.1.5)
- github.com/spf13/cobra: [v0.0.6 → v1.1.3](https://github.com/spf13/cobra/compare/v0.0.6...v1.1.3)
- github.com/spf13/viper: [v1.6.2 → v1.7.0](https://github.com/spf13/viper/compare/v1.6.2...v1.7.0)
- github.com/stretchr/testify: [v1.4.0 → v1.7.0](https://github.com/stretchr/testify/compare/v1.4.0...v1.7.0)
- github.com/tmc/grpc-websocket-proxy: [0ad062e → e5319fd](https://github.com/tmc/grpc-websocket-proxy/compare/0ad062e...e5319fd)
- github.com/vmware-tanzu/astrolabe: [v0.4.0 → 12eb18c](https://github.com/vmware-tanzu/astrolabe/compare/v0.4.0...12eb18c)
- go.etcd.io/bbolt: v1.3.3 → v1.3.6
- go.opencensus.io: v0.22.0 → v0.22.3
- go.uber.org/atomic: v1.4.0 → v1.7.0
- go.uber.org/multierr: v1.1.0 → v1.6.0
- go.uber.org/zap: v1.10.0 → v1.17.0
- golang.org/x/crypto: f7b0055 → 8b5274c
- golang.org/x/exp: c13cbed → 6cc2880
- golang.org/x/lint: 414d861 → 6edffad
- golang.org/x/mod: v0.1.0 → v0.4.2
- golang.org/x/net: 59133d7 → 37e1c6a
- golang.org/x/sync: cd5d95a → 036812b
- golang.org/x/sys: fe76b77 → 59db8d7
- golang.org/x/text: v0.3.3 → v0.3.6
- golang.org/x/time: 555d28b → 1f47c86
- golang.org/x/tools: 5eefd05 → v0.1.2
- golang.org/x/xerrors: 9bdfabe → 5ec99f8
- google.golang.org/api: v0.9.0 → v0.20.0
- google.golang.org/genproto: 8145dea → f16073e
- google.golang.org/grpc: v1.31.0 → v1.38.0
- google.golang.org/protobuf: v1.25.0 → v1.26.0
- gopkg.in/check.v1: 41f04d3 → 8fa4692
- gopkg.in/yaml.v2: v2.3.0 → v2.4.0
- gopkg.in/yaml.v3: a6ecf24 → 496545a
- honnef.co/go/tools: v0.0.1-2019.2.3 → v0.0.1-2020.1.3
- k8s.io/api: v0.18.4 → v0.22.0
- k8s.io/apiextensions-apiserver: v0.18.4 → v0.22.0
- k8s.io/apimachinery: v0.18.4 → v0.22.0
- k8s.io/apiserver: v0.18.4 → v0.22.0
- k8s.io/cli-runtime: v0.18.4 → v0.22.0
- k8s.io/client-go: v0.18.4 → v0.22.0
- k8s.io/cloud-provider: v0.18.4 → v0.22.0
- k8s.io/cluster-bootstrap: v0.18.4 → v0.22.0
- k8s.io/code-generator: v0.18.4 → v0.22.0
- k8s.io/component-base: v0.18.4 → v0.22.0
- k8s.io/cri-api: v0.18.4 → v0.22.0
- k8s.io/csi-translation-lib: v0.18.4 → v0.22.0
- k8s.io/gengo: 36b2048 → b6c5ce2
- k8s.io/klog/v2: v2.0.0 → v2.9.0
- k8s.io/kube-aggregator: v0.18.4 → v0.22.0
- k8s.io/kube-controller-manager: v0.18.4 → v0.22.0
- k8s.io/kube-openapi: 61e04a5 → 9528897
- k8s.io/kube-proxy: v0.18.4 → v0.22.0
- k8s.io/kube-scheduler: v0.18.4 → v0.22.0
- k8s.io/kubectl: v0.18.4 → v0.22.0
- k8s.io/kubelet: v0.18.4 → v0.22.0
- k8s.io/legacy-cloud-providers: v0.18.4 → v0.22.0
- k8s.io/metrics: v0.18.4 → v0.22.0
- k8s.io/sample-apiserver: v0.18.4 → v0.22.0
- k8s.io/utils: 6e3d28b → 4b05e18
- sigs.k8s.io/apiserver-network-proxy/konnectivity-client: v0.0.7 → v0.0.22
- sigs.k8s.io/structured-merge-diff/v3: v3.0.0 → 67a7b8c

#### Removed
- github.com/golangplus/bytes: [45c989f](https://github.com/golangplus/bytes/tree/45c989f)
- github.com/golangplus/fmt: [2a5d6d7](https://github.com/golangplus/fmt/tree/2a5d6d7)
- github.com/satori/go.uuid: [v1.2.0](https://github.com/satori/go.uuid/tree/v1.2.0)
- github.com/xlab/handysort: [fb3537e](https://github.com/xlab/handysort/tree/fb3537e)
- vbom.ml/util: db5cfe1
