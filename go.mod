module github.com/vmware-tanzu/velero-plugin-for-vsphere

require (
	github.com/agiledragon/gomonkey v2.0.1+incompatible
	github.com/aws/aws-sdk-go v1.29.19
	github.com/evanphx/json-patch v4.5.0+incompatible
	github.com/google/uuid v1.1.1
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.4.2
	github.com/spf13/cobra v0.0.5
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.4.0
	github.com/vmware-tanzu/astrolabe v0.1.2-0.20200818231420-1ed4b2ce0e1e
	github.com/vmware-tanzu/velero v1.3.2
	k8s.io/api v0.17.3
	k8s.io/apiextensions-apiserver v0.17.3
	k8s.io/apimachinery v0.17.3
	k8s.io/client-go v0.17.3
	k8s.io/code-generator v0.17.8 // indirect
	k8s.io/klog v1.0.0
	k8s.io/utils v0.0.0-20200324210504-a9aa75ae1b89
)

replace github.com/vmware/gvddk => ../astrolabe/vendor/github.com/vmware/gvddk

replace k8s.io/api => k8s.io/api v0.17.3

replace k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.17.3

replace k8s.io/apimachinery => k8s.io/apimachinery v0.17.3

replace k8s.io/client-go => k8s.io/client-go v0.17.3

go 1.13
