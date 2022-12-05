# Changelog since v1.3.0

## v1.4.2

Date: 2022-12-5

### Changes

- Remove volume health annotation from PVC before the backup. (#495, @xing-yang)
- Removes health annotation from PVC before handing it over to Velero during the restore. (#498, @xing-yang)

## Dependencies

### Added
- github.com/docker/spdystream: [449fdfc](https://github.com/docker/spdystream/tree/449fdfc)
- github.com/urfave/cli: [v1.20.0](https://github.com/urfave/cli/tree/v1.20.0)
- go.etcd.io/etcd: 17cef6e
- gopkg.in/cheggaaa/pb.v1: v1.0.25

### Changed
- github.com/prometheus/client_golang: [v1.11.0 → v1.11.1](https://github.com/prometheus/client_golang/compare/v1.11.0...v1.11.1)
- github.com/yuin/goldmark: [v1.3.5 → v1.4.13](https://github.com/yuin/goldmark/compare/v1.3.5...v1.4.13)
- golang.org/x/crypto: 8b5274c → 1baeb1c
- golang.org/x/mod: v0.4.2 → 86c51ed
- golang.org/x/net: 4f30a5c → f3363e0
- golang.org/x/sync: 036812b → 886fb93
- golang.org/x/sys: 7861aae → 3c1f352
- golang.org/x/term: 6a3ed07 → 03fcf44
- golang.org/x/text: v0.3.7 → v0.3.8
- golang.org/x/tools: v0.1.5 → v0.1.12
- k8s.io/api: v0.22.0 → v0.22.4
- k8s.io/apiextensions-apiserver: v0.22.0 → v0.22.4
- k8s.io/apimachinery: v0.22.0 → v0.22.4
- k8s.io/apiserver: v0.22.0 → v0.22.4
- k8s.io/cli-runtime: v0.22.0 → v0.22.2
- k8s.io/client-go: v0.22.0 → v0.22.4
- k8s.io/cluster-bootstrap: v0.22.0 → v0.22.2
- k8s.io/code-generator: v0.22.0 → v0.22.4
- k8s.io/component-base: v0.22.0 → v0.22.4
- k8s.io/component-helpers: v0.22.0 → v0.22.2
- k8s.io/kube-aggregator: v0.22.0 → v0.19.12
- k8s.io/kube-openapi: 9528897 → 2043435
- k8s.io/kubectl: v0.22.0 → v0.22.2
- k8s.io/metrics: v0.22.0 → v0.22.2

### Removed
- github.com/dgryski/go-sip13: [e10d5fe](https://github.com/dgryski/go-sip13/tree/e10d5fe)
- github.com/prometheus/tsdb: [v0.7.1](https://github.com/prometheus/tsdb/tree/v0.7.1)

## v1.4.1

Date: 2022-11-8

### Changes

- Add 1.4.0 to readme compatibility matrix. ([#480](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/pull/480), [@xinyanw409](https://github.com/xinyanw409))
- User photon image and update dependencies ([#491](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/pull/491), [@xing-yang](https://github.com/xing-yang))

## Dependencies

### Added
- github.com/cncf/xds/go: [fbca930](https://github.com/cncf/xds/go/tree/fbca930)
- github.com/coredns/caddy: [v1.1.0](https://github.com/coredns/caddy/tree/v1.1.0)
- github.com/drone/envsubst/v2: [7bf45db](https://github.com/drone/envsubst/v2/tree/7bf45db)
- github.com/go-task/slim-sprig: [348f09d](https://github.com/go-task/slim-sprig/tree/348f09d)
- github.com/golang-jwt/jwt: [v3.2.2+incompatible](https://github.com/golang-jwt/jwt/tree/v3.2.2)
- github.com/google/go-github/v33: [v33.0.0](https://github.com/google/go-github/v33/tree/v33.0.0)
- github.com/google/martian/v3: [v3.2.1](https://github.com/google/martian/v3/tree/v3.2.1)
- github.com/gosuri/uitable: [v0.0.4](https://github.com/gosuri/uitable/tree/v0.0.4)
- github.com/kr/fs: [v0.1.0](https://github.com/kr/fs/tree/v0.1.0)
- github.com/kubernetes-csi/external-snapshotter/client/v4: [v4.0.0](https://github.com/kubernetes-csi/external-snapshotter/client/v4/tree/v4.0.0)
- github.com/labstack/echo/v4: [v4.9.1](https://github.com/labstack/echo/v4/tree/v4.9.1)
- github.com/pkg/sftp: [v1.10.1](https://github.com/pkg/sftp/tree/v1.10.1)
- github.com/rivo/uniseg: [v0.2.0](https://github.com/rivo/uniseg/tree/v0.2.0)
- github.com/sagikazarmark/crypt: [v0.1.0](https://github.com/sagikazarmark/crypt/tree/v0.1.0)
- github.com/vladimirvivien/gexe: [v0.1.1](https://github.com/vladimirvivien/gexe/tree/v0.1.1)
- github.com/vmware-tanzu/crash-diagnostics: [v0.3.7](https://github.com/vmware-tanzu/crash-diagnostics/tree/v0.3.7)
- google.golang.org/grpc/cmd/protoc-gen-go-grpc: v1.1.0

### Changed
- cloud.google.com/go/bigquery: v1.4.0 → v1.8.0
- cloud.google.com/go/firestore: v1.1.0 → v1.6.0
- cloud.google.com/go/pubsub: v1.2.0 → v1.3.1
- cloud.google.com/go/storage: v1.6.0 → v1.10.0
- cloud.google.com/go: v0.54.0 → v0.93.3
- github.com/Azure/go-autorest/autorest/adal: [v0.9.13 → v0.9.14](https://github.com/Azure/go-autorest/autorest/adal/compare/v0.9.13...v0.9.14)
- github.com/Azure/go-autorest/autorest/azure/auth: [v0.4.2 → v0.5.8](https://github.com/Azure/go-autorest/autorest/azure/auth/compare/v0.4.2...v0.5.8)
- github.com/Azure/go-autorest/autorest/azure/cli: [v0.3.1 → v0.4.2](https://github.com/Azure/go-autorest/autorest/azure/cli/compare/v0.3.1...v0.4.2)
- github.com/Azure/go-autorest/autorest: [v0.11.18 → v0.11.21](https://github.com/Azure/go-autorest/autorest/compare/v0.11.18...v0.11.21)
- github.com/armon/go-radix: [7fddfc3 → v1.0.0](https://github.com/armon/go-radix/compare/7fddfc3...v1.0.0)
- github.com/aws/aws-sdk-go: [v1.38.49 → v1.42.10](https://github.com/aws/aws-sdk-go/compare/v1.38.49...v1.42.10)
- github.com/benbjohnson/clock: [v1.0.3 → v1.1.0](https://github.com/benbjohnson/clock/compare/v1.0.3...v1.1.0)
- github.com/bketelsen/crypt: [5cbc8cc → v0.0.4](https://github.com/bketelsen/crypt/compare/5cbc8cc...v0.0.4)
- github.com/coredns/corefile-migration: [v1.0.7 → v1.0.13](https://github.com/coredns/corefile-migration/compare/v1.0.7...v1.0.13)
- github.com/dimchansky/utfbom: [v1.1.0 → v1.1.1](https://github.com/dimchansky/utfbom/compare/v1.1.0...v1.1.1)
- github.com/envoyproxy/go-control-plane: [668b12f → 63b5d3c](https://github.com/envoyproxy/go-control-plane/compare/668b12f...63b5d3c)
- github.com/fatih/color: [v1.7.0 → v1.13.0](https://github.com/fatih/color/compare/v1.7.0...v1.13.0)
- github.com/fsnotify/fsnotify: [v1.4.9 → v1.5.1](https://github.com/fsnotify/fsnotify/compare/v1.4.9...v1.5.1)
- github.com/go-logr/zapr: [v0.1.0 → v0.4.0](https://github.com/go-logr/zapr/compare/v0.1.0...v0.4.0)
- github.com/gobuffalo/flect: [v0.1.3 → v0.2.3](https://github.com/gobuffalo/flect/compare/v0.1.3...v0.2.3)
- github.com/golang/mock: [v1.4.4 → v1.6.0](https://github.com/golang/mock/compare/v1.4.4...v1.6.0)
- github.com/golang/snappy: [v0.0.1 → v0.0.3](https://github.com/golang/snappy/compare/v0.0.1...v0.0.3)
- github.com/google/go-cmp: [v0.5.5 → v0.5.6](https://github.com/google/go-cmp/compare/v0.5.5...v0.5.6)
- github.com/google/gofuzz: [v1.1.0 → v1.2.0](https://github.com/google/gofuzz/compare/v1.1.0...v1.2.0)
- github.com/google/pprof: [1ebb73c → 4bb14d4](https://github.com/google/pprof/compare/1ebb73c...4bb14d4)
- github.com/googleapis/gax-go/v2: [v2.0.5 → v2.1.0](https://github.com/googleapis/gax-go/v2/compare/v2.0.5...v2.1.0)
- github.com/hashicorp/consul/api: [v1.1.0 → v1.10.1](https://github.com/hashicorp/consul/api/compare/v1.1.0...v1.10.1)
- github.com/hashicorp/consul/sdk: [v0.1.1 → v0.8.0](https://github.com/hashicorp/consul/sdk/compare/v0.1.1...v0.8.0)
- github.com/hashicorp/go-hclog: [v0.8.0 → v0.12.0](https://github.com/hashicorp/go-hclog/compare/v0.8.0...v0.12.0)
- github.com/hashicorp/go-multierror: [v1.0.0 → v1.1.0](https://github.com/hashicorp/go-multierror/compare/v1.0.0...v1.1.0)
- github.com/hashicorp/go-rootcerts: [v1.0.0 → v1.0.2](https://github.com/hashicorp/go-rootcerts/compare/v1.0.0...v1.0.2)
- github.com/hashicorp/golang-lru: [v0.5.4 → v0.5.1](https://github.com/hashicorp/golang-lru/compare/v0.5.4...v0.5.1)
- github.com/hashicorp/mdns: [v1.0.0 → v1.0.1](https://github.com/hashicorp/mdns/compare/v1.0.0...v1.0.1)
- github.com/hashicorp/memberlist: [v0.1.3 → v0.2.2](https://github.com/hashicorp/memberlist/compare/v0.1.3...v0.2.2)
- github.com/hashicorp/serf: [v0.8.2 → v0.9.5](https://github.com/hashicorp/serf/compare/v0.8.2...v0.9.5)
- github.com/ianlancetaylor/demangle: [5e5cf60 → 28f6c0f](https://github.com/ianlancetaylor/demangle/compare/5e5cf60...28f6c0f)
- github.com/imdario/mergo: [v0.3.9 → v0.3.12](https://github.com/imdario/mergo/compare/v0.3.9...v0.3.12)
- github.com/labstack/gommon: [v0.3.0 → v0.4.0](https://github.com/labstack/gommon/compare/v0.3.0...v0.4.0)
- github.com/magiconair/properties: [v1.8.1 → v1.8.5](https://github.com/magiconair/properties/compare/v1.8.1...v1.8.5)
- github.com/mattn/go-colorable: [v0.1.2 → v0.1.11](https://github.com/mattn/go-colorable/compare/v0.1.2...v0.1.11)
- github.com/mattn/go-isatty: [v0.0.12 → v0.0.14](https://github.com/mattn/go-isatty/compare/v0.0.12...v0.0.14)
- github.com/mattn/go-runewidth: [v0.0.7 → v0.0.13](https://github.com/mattn/go-runewidth/compare/v0.0.7...v0.0.13)
- github.com/miekg/dns: [v1.1.4 → v1.1.26](https://github.com/miekg/dns/compare/v1.1.4...v1.1.26)
- github.com/mitchellh/cli: [v1.0.0 → v1.1.0](https://github.com/mitchellh/cli/compare/v1.0.0...v1.1.0)
- github.com/mitchellh/mapstructure: [v1.4.1 → v1.4.2](https://github.com/mitchellh/mapstructure/compare/v1.4.1...v1.4.2)
- github.com/nxadm/tail: [v1.4.4 → v1.4.8](https://github.com/nxadm/tail/compare/v1.4.4...v1.4.8)
- github.com/onsi/ginkgo: [v1.14.0 → v1.16.4](https://github.com/onsi/ginkgo/compare/v1.14.0...v1.16.4)
- github.com/onsi/gomega: [v1.10.1 → v1.16.0](https://github.com/onsi/gomega/compare/v1.10.1...v1.16.0)
- github.com/pelletier/go-toml: [v1.7.0 → v1.9.4](https://github.com/pelletier/go-toml/compare/v1.7.0...v1.9.4)
- github.com/posener/complete: [v1.1.1 → v1.2.3](https://github.com/posener/complete/compare/v1.1.1...v1.2.3)
- github.com/spf13/afero: [v1.2.2 → v1.6.0](https://github.com/spf13/afero/compare/v1.2.2...v1.6.0)
- github.com/spf13/cast: [v1.3.0 → v1.4.1](https://github.com/spf13/cast/compare/v1.3.0...v1.4.1)
- github.com/spf13/cobra: [v1.1.3 → v1.2.1](https://github.com/spf13/cobra/compare/v1.1.3...v1.2.1)
- github.com/spf13/viper: [v1.7.0 → v1.9.0](https://github.com/spf13/viper/compare/v1.7.0...v1.9.0)
- github.com/valyala/fasttemplate: [v1.0.1 → v1.2.1](https://github.com/valyala/fasttemplate/compare/v1.0.1...v1.2.1)
- github.com/vmware-tanzu/astrolabe: [v0.5.1 → b206312](https://github.com/vmware-tanzu/astrolabe/compare/v0.5.1...b206312)
- github.com/vmware-tanzu/velero: [v1.5.1 → v1.7.1](https://github.com/vmware-tanzu/velero/compare/v1.5.1...v1.7.1)
- go.opencensus.io: v0.22.3 → v0.23.0
- go.starlark.net: 8dd3e2e → 227f4aa
- go.uber.org/zap: v1.17.0 → v1.19.0
- golang.org/x/net: 37e1c6a → 4f30a5c
- golang.org/x/oauth2: bf48bf1 → 2bc19b1
- golang.org/x/sys: 59db8d7 → 7861aae
- golang.org/x/tools: v0.1.2 → v0.1.5
- gomodules.xyz/jsonpatch/v2: v2.0.1 → v2.2.0
- google.golang.org/api: v0.20.0 → v0.56.0
- google.golang.org/appengine: v1.6.6 → v1.6.7
- google.golang.org/genproto: f16073e → 66f60bf
- google.golang.org/grpc: v1.38.0 → v1.40.0
- google.golang.org/protobuf: v1.26.0 → v1.27.1
- gopkg.in/ini.v1: v1.51.0 → v1.63.2
- honnef.co/go/tools: v0.0.1-2020.1.3 → v0.0.1-2020.1.4
- k8s.io/utils: 4b05e18 → cb0fa31
- sigs.k8s.io/cluster-api: v0.3.8 → v1.0.0
- sigs.k8s.io/controller-runtime: v0.6.1 → v0.10.2
- sigs.k8s.io/yaml: v1.2.0 → v1.3.0

### Removed
- bitbucket.org/bertimus9/systemstat: 0eeff89
- github.com/GoogleCloudPlatform/k8s-cloud-provider: [7901bc8](https://github.com/GoogleCloudPlatform/k8s-cloud-provider/tree/7901bc8)
- github.com/JeffAshton/win_pdh: [76bb4ee](https://github.com/JeffAshton/win_pdh/tree/76bb4ee)
- github.com/Microsoft/go-winio: [v0.4.14](https://github.com/Microsoft/go-winio/tree/v0.4.14)
- github.com/Microsoft/hcsshim: [672e52e](https://github.com/Microsoft/hcsshim/tree/672e52e)
- github.com/OpenPeeDeeP/depguard: [v1.0.1](https://github.com/OpenPeeDeeP/depguard/tree/v1.0.1)
- github.com/Rican7/retry: [v0.1.0](https://github.com/Rican7/retry/tree/v0.1.0)
- github.com/StackExchange/wmi: [5d04971](https://github.com/StackExchange/wmi/tree/5d04971)
- github.com/ajstarks/svgo: [644b8db](https://github.com/ajstarks/svgo/tree/644b8db)
- github.com/alessio/shellescape: [b115ca0](https://github.com/alessio/shellescape/tree/b115ca0)
- github.com/anmitsu/go-shlex: [648efa6](https://github.com/anmitsu/go-shlex/tree/648efa6)
- github.com/auth0/go-jwt-middleware: [5493cab](https://github.com/auth0/go-jwt-middleware/tree/5493cab)
- github.com/bazelbuild/bazel-gazelle: [70208cb](https://github.com/bazelbuild/bazel-gazelle/tree/70208cb)
- github.com/bazelbuild/buildtools: [69366ca](https://github.com/bazelbuild/buildtools/tree/69366ca)
- github.com/bazelbuild/rules_go: [6dae44d](https://github.com/bazelbuild/rules_go/tree/6dae44d)
- github.com/bifurcation/mint: [93c51c6](https://github.com/bifurcation/mint/tree/93c51c6)
- github.com/boltdb/bolt: [v1.3.1](https://github.com/boltdb/bolt/tree/v1.3.1)
- github.com/bradfitz/go-smtpd: [deb6d62](https://github.com/bradfitz/go-smtpd/tree/deb6d62)
- github.com/caddyserver/caddy: [v1.0.3](https://github.com/caddyserver/caddy/tree/v1.0.3)
- github.com/cenkalti/backoff: [v2.1.1+incompatible](https://github.com/cenkalti/backoff/tree/v2.1.1)
- github.com/cespare/prettybench: [03b8cfe](https://github.com/cespare/prettybench/tree/03b8cfe)
- github.com/checkpoint-restore/go-criu: [17b0214](https://github.com/checkpoint-restore/go-criu/tree/17b0214)
- github.com/cheekybits/genny: [9127e81](https://github.com/cheekybits/genny/tree/9127e81)
- github.com/cilium/ebpf: [95b36a5](https://github.com/cilium/ebpf/tree/95b36a5)
- github.com/clusterhq/flocker-go: [2b8b725](https://github.com/clusterhq/flocker-go/tree/2b8b725)
- github.com/codegangsta/negroni: [v1.0.0](https://github.com/codegangsta/negroni/tree/v1.0.0)
- github.com/container-storage-interface/spec: [v1.2.0](https://github.com/container-storage-interface/spec/tree/v1.2.0)
- github.com/containerd/console: [84eeaae](https://github.com/containerd/console/tree/84eeaae)
- github.com/containerd/containerd: [v1.0.2](https://github.com/containerd/containerd/tree/v1.0.2)
- github.com/containerd/typeurl: [2a93cfd](https://github.com/containerd/typeurl/tree/2a93cfd)
- github.com/containernetworking/cni: [v0.7.1](https://github.com/containernetworking/cni/tree/v0.7.1)
- github.com/coreos/go-etcd: [v2.0.0+incompatible](https://github.com/coreos/go-etcd/tree/v2.0.0)
- github.com/cpuguy83/go-md2man: [v1.0.10](https://github.com/cpuguy83/go-md2man/tree/v1.0.10)
- github.com/cyphar/filepath-securejoin: [v0.2.2](https://github.com/cyphar/filepath-securejoin/tree/v0.2.2)
- github.com/dnaeon/go-vcr: [v1.0.1](https://github.com/dnaeon/go-vcr/tree/v1.0.1)
- github.com/docker/docker: [be7ac8b](https://github.com/docker/docker/tree/be7ac8b)
- github.com/docker/go-connections: [v0.3.0](https://github.com/docker/go-connections/tree/v0.3.0)
- github.com/docker/libnetwork: [c8a5fca](https://github.com/docker/libnetwork/tree/c8a5fca)
- github.com/docker/spdystream: [bc6354c](https://github.com/docker/spdystream/tree/bc6354c)
- github.com/drone/envsubst: [efdb65b](https://github.com/drone/envsubst/tree/efdb65b)
- github.com/euank/go-kmsg-parser: [v2.0.0+incompatible](https://github.com/euank/go-kmsg-parser/tree/v2.0.0)
- github.com/fogleman/gg: [0403632](https://github.com/fogleman/gg/tree/0403632)
- github.com/gliderlabs/ssh: [v0.1.1](https://github.com/gliderlabs/ssh/tree/v0.1.1)
- github.com/go-acme/lego: [v2.5.0+incompatible](https://github.com/go-acme/lego/tree/v2.5.0)
- github.com/go-bindata/go-bindata: [v3.1.1+incompatible](https://github.com/go-bindata/go-bindata/tree/v3.1.1)
- github.com/go-critic/go-critic: [1df3008](https://github.com/go-critic/go-critic/tree/1df3008)
- github.com/go-lintpack/lintpack: [v0.5.2](https://github.com/go-lintpack/lintpack/tree/v0.5.2)
- github.com/go-ole/go-ole: [v1.2.1](https://github.com/go-ole/go-ole/tree/v1.2.1)
- github.com/go-ozzo/ozzo-validation: [v3.5.0+incompatible](https://github.com/go-ozzo/ozzo-validation/tree/v3.5.0)
- github.com/go-toolsmith/astcast: [v1.0.0](https://github.com/go-toolsmith/astcast/tree/v1.0.0)
- github.com/go-toolsmith/astcopy: [v1.0.0](https://github.com/go-toolsmith/astcopy/tree/v1.0.0)
- github.com/go-toolsmith/astequal: [v1.0.0](https://github.com/go-toolsmith/astequal/tree/v1.0.0)
- github.com/go-toolsmith/astfmt: [v1.0.0](https://github.com/go-toolsmith/astfmt/tree/v1.0.0)
- github.com/go-toolsmith/astinfo: [9809ff7](https://github.com/go-toolsmith/astinfo/tree/9809ff7)
- github.com/go-toolsmith/astp: [v1.0.0](https://github.com/go-toolsmith/astp/tree/v1.0.0)
- github.com/go-toolsmith/pkgload: [v1.0.0](https://github.com/go-toolsmith/pkgload/tree/v1.0.0)
- github.com/go-toolsmith/strparse: [v1.0.0](https://github.com/go-toolsmith/strparse/tree/v1.0.0)
- github.com/go-toolsmith/typep: [v1.0.0](https://github.com/go-toolsmith/typep/tree/v1.0.0)
- github.com/godbus/dbus: [2ff6f7f](https://github.com/godbus/dbus/tree/2ff6f7f)
- github.com/golang/freetype: [e2365df](https://github.com/golang/freetype/tree/e2365df)
- github.com/golangci/check: [cfe4005](https://github.com/golangci/check/tree/cfe4005)
- github.com/golangci/dupl: [3e9179a](https://github.com/golangci/dupl/tree/3e9179a)
- github.com/golangci/errcheck: [ef45e06](https://github.com/golangci/errcheck/tree/ef45e06)
- github.com/golangci/go-misc: [927a3d8](https://github.com/golangci/go-misc/tree/927a3d8)
- github.com/golangci/go-tools: [e32c541](https://github.com/golangci/go-tools/tree/e32c541)
- github.com/golangci/goconst: [041c5f2](https://github.com/golangci/goconst/tree/041c5f2)
- github.com/golangci/gocyclo: [2becd97](https://github.com/golangci/gocyclo/tree/2becd97)
- github.com/golangci/gofmt: [0b8337e](https://github.com/golangci/gofmt/tree/0b8337e)
- github.com/golangci/golangci-lint: [v1.18.0](https://github.com/golangci/golangci-lint/tree/v1.18.0)
- github.com/golangci/gosec: [66fb7fc](https://github.com/golangci/gosec/tree/66fb7fc)
- github.com/golangci/ineffassign: [42439a7](https://github.com/golangci/ineffassign/tree/42439a7)
- github.com/golangci/lint-1: [ee948d0](https://github.com/golangci/lint-1/tree/ee948d0)
- github.com/golangci/maligned: [b1d8939](https://github.com/golangci/maligned/tree/b1d8939)
- github.com/golangci/misspell: [950f5d1](https://github.com/golangci/misspell/tree/950f5d1)
- github.com/golangci/prealloc: [215b22d](https://github.com/golangci/prealloc/tree/215b22d)
- github.com/golangci/revgrep: [d9c87f5](https://github.com/golangci/revgrep/tree/d9c87f5)
- github.com/golangci/unconvert: [28b1c44](https://github.com/golangci/unconvert/tree/28b1c44)
- github.com/google/cadvisor: [v0.35.0](https://github.com/google/cadvisor/tree/v0.35.0)
- github.com/google/go-github: [v17.0.0+incompatible](https://github.com/google/go-github/tree/v17.0.0)
- github.com/gophercloud/gophercloud: [v0.1.0](https://github.com/gophercloud/gophercloud/tree/v0.1.0)
- github.com/gorilla/context: [v1.1.1](https://github.com/gorilla/context/tree/v1.1.1)
- github.com/gorilla/mux: [v1.7.0](https://github.com/gorilla/mux/tree/v1.7.0)
- github.com/gostaticanalysis/analysisutil: [v0.0.3](https://github.com/gostaticanalysis/analysisutil/tree/v0.0.3)
- github.com/heketi/heketi: [c2e2a4a](https://github.com/heketi/heketi/tree/c2e2a4a)
- github.com/heketi/tests: [f3775cb](https://github.com/heketi/tests/tree/f3775cb)
- github.com/jellevandenhooff/dkim: [f50fe3d](https://github.com/jellevandenhooff/dkim/tree/f50fe3d)
- github.com/jimstudt/http-authentication: [3eca13d](https://github.com/jimstudt/http-authentication/tree/3eca13d)
- github.com/jung-kurt/gofpdf: [24315ac](https://github.com/jung-kurt/gofpdf/tree/24315ac)
- github.com/klauspost/cpuid: [v1.2.0](https://github.com/klauspost/cpuid/tree/v1.2.0)
- github.com/kubernetes-csi/csi-lib-utils: [v0.7.0](https://github.com/kubernetes-csi/csi-lib-utils/tree/v0.7.0)
- github.com/kubernetes-csi/csi-test: [v2.0.0+incompatible](https://github.com/kubernetes-csi/csi-test/tree/v2.0.0)
- github.com/kubernetes-csi/external-snapshotter/v2: [v2.2.0-rc1](https://github.com/kubernetes-csi/external-snapshotter/v2/tree/v2.2.0-rc1)
- github.com/kylelemons/godebug: [d65d576](https://github.com/kylelemons/godebug/tree/d65d576)
- github.com/labstack/echo: [v3.3.10+incompatible](https://github.com/labstack/echo/tree/v3.3.10)
- github.com/libopenstorage/openstorage: [v1.0.0](https://github.com/libopenstorage/openstorage/tree/v1.0.0)
- github.com/logrusorgru/aurora: [a7b3b31](https://github.com/logrusorgru/aurora/tree/a7b3b31)
- github.com/lpabon/godbc: [v0.1.1](https://github.com/lpabon/godbc/tree/v0.1.1)
- github.com/lucas-clemente/aes12: [cd47fb3](https://github.com/lucas-clemente/aes12/tree/cd47fb3)
- github.com/lucas-clemente/quic-clients: [v0.1.0](https://github.com/lucas-clemente/quic-clients/tree/v0.1.0)
- github.com/lucas-clemente/quic-go-certificates: [d2f8652](https://github.com/lucas-clemente/quic-go-certificates/tree/d2f8652)
- github.com/lucas-clemente/quic-go: [v0.10.2](https://github.com/lucas-clemente/quic-go/tree/v0.10.2)
- github.com/marten-seemann/qtls: [v0.2.3](https://github.com/marten-seemann/qtls/tree/v0.2.3)
- github.com/mattn/go-shellwords: [v1.0.5](https://github.com/mattn/go-shellwords/tree/v1.0.5)
- github.com/mattn/goveralls: [v0.0.2](https://github.com/mattn/goveralls/tree/v0.0.2)
- github.com/mesos/mesos-go: [v0.0.9](https://github.com/mesos/mesos-go/tree/v0.0.9)
- github.com/mholt/certmagic: [6a42ef9](https://github.com/mholt/certmagic/tree/6a42ef9)
- github.com/mindprince/gonvml: [9ebdce4](https://github.com/mindprince/gonvml/tree/9ebdce4)
- github.com/mistifyio/go-zfs: [v2.1.1+incompatible](https://github.com/mistifyio/go-zfs/tree/v2.1.1)
- github.com/mitchellh/go-ps: [4fdf99a](https://github.com/mitchellh/go-ps/tree/4fdf99a)
- github.com/mohae/deepcopy: [491d360](https://github.com/mohae/deepcopy/tree/491d360)
- github.com/morikuni/aec: [v1.0.0](https://github.com/morikuni/aec/tree/v1.0.0)
- github.com/mozilla/tls-observatory: [8791a20](https://github.com/mozilla/tls-observatory/tree/8791a20)
- github.com/mrunalp/fileutils: [7d4729f](https://github.com/mrunalp/fileutils/tree/7d4729f)
- github.com/mvdan/xurls: [v1.1.0](https://github.com/mvdan/xurls/tree/v1.1.0)
- github.com/naoina/go-stringutil: [v0.1.0](https://github.com/naoina/go-stringutil/tree/v0.1.0)
- github.com/naoina/toml: [v0.1.1](https://github.com/naoina/toml/tree/v0.1.1)
- github.com/nbutton23/zxcvbn-go: [eafdab6](https://github.com/nbutton23/zxcvbn-go/tree/eafdab6)
- github.com/opencontainers/image-spec: [v1.0.1](https://github.com/opencontainers/image-spec/tree/v1.0.1)
- github.com/opencontainers/runc: [v1.0.0-rc10](https://github.com/opencontainers/runc/tree/v1.0.0-rc10)
- github.com/opencontainers/runtime-spec: [v1.0.0](https://github.com/opencontainers/runtime-spec/tree/v1.0.0)
- github.com/opencontainers/selinux: [5215b18](https://github.com/opencontainers/selinux/tree/5215b18)
- github.com/pquerna/ffjson: [af8b230](https://github.com/pquerna/ffjson/tree/af8b230)
- github.com/quasilyte/go-consistent: [c6f3937](https://github.com/quasilyte/go-consistent/tree/c6f3937)
- github.com/quobyte/api: [v0.1.2](https://github.com/quobyte/api/tree/v0.1.2)
- github.com/remyoudompheng/bigfft: [52369c6](https://github.com/remyoudompheng/bigfft/tree/52369c6)
- github.com/rubiojr/go-vhd: [02e2102](https://github.com/rubiojr/go-vhd/tree/02e2102)
- github.com/ryanuber/go-glob: [256dc44](https://github.com/ryanuber/go-glob/tree/256dc44)
- github.com/sclevine/agouti: [v3.0.0+incompatible](https://github.com/sclevine/agouti/tree/v3.0.0)
- github.com/seccomp/libseccomp-golang: [v0.9.1](https://github.com/seccomp/libseccomp-golang/tree/v0.9.1)
- github.com/shirou/gopsutil: [c95755e](https://github.com/shirou/gopsutil/tree/c95755e)
- github.com/shirou/w32: [bb4de01](https://github.com/shirou/w32/tree/bb4de01)
- github.com/shurcooL/go-goon: [37c2f52](https://github.com/shurcooL/go-goon/tree/37c2f52)
- github.com/shurcooL/go: [9e1955d](https://github.com/shurcooL/go/tree/9e1955d)
- github.com/sourcegraph/go-diff: [v0.5.1](https://github.com/sourcegraph/go-diff/tree/v0.5.1)
- github.com/storageos/go-api: [343b3ef](https://github.com/storageos/go-api/tree/343b3ef)
- github.com/syndtr/gocapability: [d983527](https://github.com/syndtr/gocapability/tree/d983527)
- github.com/tarm/serial: [98f6abe](https://github.com/tarm/serial/tree/98f6abe)
- github.com/thecodeteam/goscaleio: [v0.1.0](https://github.com/thecodeteam/goscaleio/tree/v0.1.0)
- github.com/timakin/bodyclose: [87058b9](https://github.com/timakin/bodyclose/tree/87058b9)
- github.com/ugorji/go/codec: [d75b2dc](https://github.com/ugorji/go/codec/tree/d75b2dc)
- github.com/ultraware/funlen: [v0.0.2](https://github.com/ultraware/funlen/tree/v0.0.2)
- github.com/urfave/cli: [v1.20.0](https://github.com/urfave/cli/tree/v1.20.0)
- github.com/urfave/negroni: [v1.0.0](https://github.com/urfave/negroni/tree/v1.0.0)
- github.com/valyala/fasthttp: [v1.2.0](https://github.com/valyala/fasthttp/tree/v1.2.0)
- github.com/valyala/quicktemplate: [v1.1.1](https://github.com/valyala/quicktemplate/tree/v1.1.1)
- github.com/valyala/tcplisten: [ceec8f9](https://github.com/valyala/tcplisten/tree/ceec8f9)
- github.com/vishvananda/netlink: [v1.0.0](https://github.com/vishvananda/netlink/tree/v1.0.0)
- github.com/vishvananda/netns: [be1fbed](https://github.com/vishvananda/netns/tree/be1fbed)
- go.etcd.io/etcd: 3cf2f69
- go4.org: 417644f
- golang.org/x/build: 2835ba2
- golang.org/x/perf: 6e6d33e
- gonum.org/v1/gonum: v0.6.2
- gonum.org/v1/netlib: 7672324
- gonum.org/v1/plot: e2840ee
- gopkg.in/airbrake/gobrake.v2: v2.0.9
- gopkg.in/cheggaaa/pb.v1: v1.0.25
- gopkg.in/gcfg.v1: v1.2.0
- gopkg.in/gemnasium/logrus-airbrake-hook.v2: v2.1.2
- gopkg.in/mcuadros/go-syslog.v2: v2.2.1
- gopkg.in/warnings.v0: v0.1.1
- gotest.tools/gotestsum: v0.3.5
- grpc.go4.org: 11d0a25
- k8s.io/cloud-provider: v0.22.0
- k8s.io/controller-manager: v0.22.0
- k8s.io/cri-api: v0.22.0
- k8s.io/csi-translation-lib: v0.22.0
- k8s.io/heapster: v1.2.0-beta.1
- k8s.io/kube-controller-manager: v0.22.0
- k8s.io/kube-proxy: v0.22.0
- k8s.io/kube-scheduler: v0.22.0
- k8s.io/kubelet: v0.22.0
- k8s.io/kubernetes: v1.18.0
- k8s.io/legacy-cloud-providers: v0.22.0
- k8s.io/mount-utils: v0.22.0
- k8s.io/repo-infra: v0.0.1-alpha.1
- k8s.io/sample-apiserver: v0.22.0
- k8s.io/system-validators: v1.0.4
- modernc.org/cc: v1.0.0
- modernc.org/golex: v1.0.0
- modernc.org/mathutil: v1.0.0
- modernc.org/strutil: v1.0.0
- modernc.org/xc: v1.0.0
- mvdan.cc/interfacer: c200402
- mvdan.cc/lint: adc824a
- mvdan.cc/unparam: fbb5962
- rsc.io/pdf: v0.1.1
- sigs.k8s.io/kind: 981bd80
- sigs.k8s.io/kustomize: v2.0.3+incompatible
- sigs.k8s.io/structured-merge-diff/v3: v3.0.0
- sourcegraph.com/sqs/pbtypes: d3ebe8f

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
