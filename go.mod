module github.com/vmware-tanzu/velero-plugin-for-vsphere

require (
	github.com/Azure/azure-sdk-for-go v35.0.0+incompatible // indirect
	github.com/Azure/go-autorest/autorest/validation v0.2.0 // indirect
	github.com/agiledragon/gomonkey v2.0.1+incompatible
	github.com/aws/aws-sdk-go v1.29.19
	github.com/evanphx/json-patch v4.5.0+incompatible
	github.com/fsnotify/fsnotify v1.4.9 // indirect
	github.com/gobwas/glob v0.2.3 // indirect
	github.com/gofrs/uuid v3.2.0+incompatible // indirect
	github.com/gogo/protobuf v1.3.1 // indirect
	github.com/golang/groupcache v0.0.0-20190129154638-5b532d6fd5ef // indirect
	github.com/google/uuid v1.1.1
	github.com/hashicorp/go-hclog v0.8.0 // indirect
	github.com/hashicorp/go-plugin v0.0.0-20190220160451-3f118e8ee104 // indirect
	github.com/hashicorp/yamux v0.0.0-20181012175058-2f1d1f20f75d // indirect
	github.com/json-iterator/go v1.1.9 // indirect
	github.com/mitchellh/go-testing-interface v1.0.0 // indirect
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.4.2
	github.com/spf13/cobra v0.0.5
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.4.0
	github.com/vmware-tanzu/astrolabe v0.1.2-0.20200831172741-bb29bb208305
	github.com/vmware-tanzu/velero v1.3.2
	gotest.tools v2.2.0+incompatible
	k8s.io/api v0.17.3
	k8s.io/apiextensions-apiserver v0.17.3
	k8s.io/apimachinery v0.17.3
	k8s.io/cli-runtime v0.17.3 // indirect
	k8s.io/client-go v0.17.3
	k8s.io/klog v1.0.0
	k8s.io/utils v0.0.0-20200229041039-0a110f9eb7ab
)

replace github.com/vmware/gvddk => ../astrolabe/vendor/github.com/vmware/gvddk

go 1.13
