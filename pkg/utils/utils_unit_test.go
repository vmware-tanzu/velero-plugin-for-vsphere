/*
Copyright 2020 the Velero contributors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package utils

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/constants"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	kubeclientfake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"

	"github.com/agiledragon/gomonkey"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	backupdriverapi "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/backupdriver/v1alpha1"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned/fake"
	backupdriverTypedV1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned/typed/backupdriver/v1alpha1"
	informers "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/informers/externalversions"
	veleroplugintest "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/test"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	decoupleVSphereCSIDriverFeatureEnabled = &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.VSpherePluginFeatureStates,
			Namespace: constants.DefaultVeleroNamespace,
		},
		Data: map[string]string{
			"decouple-vsphere-csi-driver": "true",
			"local-mode":                  "false",
		},
	}
	decoupleVSphereCSIDriverFeatureDisabled = &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.VSpherePluginFeatureStates,
			Namespace: constants.DefaultVeleroNamespace,
		},
		Data: map[string]string{
			"decouple-vsphere-csi-driver": "false",
			"local-mode":                  "false",
		},
	}
	csi230VSphereCredentialSecret = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vsphere-config-secret",
			Namespace: "vmware-system-csi",
		},
		Data: map[string][]byte{
			"csi-vsphere.conf": []byte(vcCredentials),
		},
		StringData: nil,
		Type:       "Opaque",
	}
	csi201VSphereCredentialSecret = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vsphere-config-secret",
			Namespace: "kube-system",
		},
		Data: map[string][]byte{
			"csi-vsphere.conf": []byte(vcCredentials),
		},
		StringData: nil,
		Type:       "Opaque",
	}
	pluginVSphereCredentialSecret = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "velero-vsphere-config-secret",
			Namespace: "velero",
		},
		Data: map[string][]byte{
			"csi-vsphere.conf": []byte(vcCredentials),
		},
		StringData: nil,
		Type:       "Opaque",
	}
	pluginConfigVanilla = &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.VeleroVSpherePluginConfig,
			Namespace: "velero",
		},
		Data: map[string]string{
			"cluster_flavor":           "VANILLA",
			"vsphere_secret_name":      "velero-vsphere-config-secret",
			"vsphere_secret_namespace": "velero",
		},
	}
	pluginConfigVanillaSecretUnspecified = &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.VeleroVSpherePluginConfig,
			Namespace: "velero",
		},
		Data: map[string]string{
			"cluster_flavor": "VANILLA",
		},
	}

	pluginConfigVanillaWithUploadCRRetryMax = &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.VeleroVSpherePluginConfig,
			Namespace: "velero",
		},
		Data: map[string]string{
			"cluster_flavor":           "VANILLA",
			"vsphere_secret_name":      "velero-vsphere-config-secret",
			"vsphere_secret_namespace": "velero",
			"upload-cr-retry-max":      "5",
		},
	}

	pluginConfigGuest = &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.VeleroVSpherePluginConfig,
			Namespace: "velero",
		},
		Data: map[string]string{
			"cluster_flavor":           "GUEST",
			"vsphere_secret_name":      "velero-vsphere-config-secret",
			"vsphere_secret_namespace": "velero",
		},
	}
	pluginConfigInvalid = &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      constants.VeleroVSpherePluginConfig,
			Namespace: "velero",
		},
		Data: map[string]string{
			"cluster_flavor": "INVALID",
		},
	}
	csi230GuestDeployment = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "vmware-system-csi",
			Name:      "vsphere-csi-controller",
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "vsphere-csi-controller",
							Image: "gcr.io/cloud-provider-vsphere/csi/release/driver:v2.3.0",
							Env: []corev1.EnvVar{
								{
									Name:  "CLUSTER_FLAVOR",
									Value: "GUEST_CLUSTER",
								},
							},
						},
					},
				},
			},
		},
	}
	csi230SupervisorDeployment = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "vmware-system-csi",
			Name:      "vsphere-csi-controller",
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "vsphere-csi-controller",
							Image: "gcr.io/cloud-provider-vsphere/csi/release/driver:v2.3.0",
							Env: []corev1.EnvVar{
								{
									Name:  "CLUSTER_FLAVOR",
									Value: "WORKLOAD",
								},
							},
						},
					},
				},
			},
		},
	}
	csi201VanillaDeployment = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "kube-system",
			Name:      "vsphere-csi-controller",
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "vsphere-csi-controller",
							Image: "gcr.io/cloud-provider-vsphere/csi/release/driver:v2.0.1",
							Env: []corev1.EnvVar{
								{
									Name:  "",
									Value: "",
								},
							},
						},
					},
				},
			},
		},
	}
	csi230VanillaDeployment = &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "vmware-system-csi",
			Name:      "vsphere-csi-controller",
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "vsphere-csi-controller",
							Image: "gcr.io/cloud-provider-vsphere/csi/release/driver:v2.3.0",
							Env: []corev1.EnvVar{
								{
									Name:  "",
									Value: "",
								},
							},
						},
					},
				},
			},
		},
	}
	csi102StatefultSet = &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "kube-system",
			Name:      "vsphere-csi-controller",
		},
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "vsphere-csi-controller",
							Image: "xyz.io:9999/cloud-provider-vsphere/csi/release/driver:v1.0.3",
						},
					},
				},
			},
		},
	}
	vcCredentials = `[Global]
insecure-flag = "true"
cluster-id = "cluster1"
cluster-distribution = "CSI-Vanilla"

[VirtualCenter "10.182.1.133"]
user = "Administrator@vsphere.local"
password = "6^54#,RDvwgJ\\nEdg$2"
datacenters = "VSAN-DC"
port = "443"
	`
)

func TestGetStringFromParamsMap(t *testing.T) {
	params := make(map[string]interface{})
	params["ValidKey"] = "ValidValue"
	params["NonString"] = false
	tests := []struct {
		name          string
		key           string
		expectedValue string
		ok            bool
	}{
		{
			name:          "Valid key with string value should return corresponding value and true",
			key:           "ValidKey",
			expectedValue: "ValidValue",
			ok:            true,
		},
		{
			name:          "If value in map is non-string should return empty string and false",
			key:           "NonString",
			expectedValue: "",
			ok:            false,
		},
		{
			name:          "No such key should return empty string and false",
			key:           "NoSuchKey",
			expectedValue: "",
			ok:            false,
		},
	}

	logger := veleroplugintest.NewLogger()

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			str, ok := GetStringFromParamsMap(params, test.key, logger)
			assert.Equal(t, test.expectedValue, str)
			assert.Equal(t, test.ok, ok)
		})
	}
}

func TestGetBool(t *testing.T) {
	tests := []struct {
		name        string
		str         string
		defValue    bool
		expectedVal bool
	}{
		{
			name:        "Pos1",
			str:         "true",
			defValue:    false,
			expectedVal: true,
		},
		{
			name:        "Pos2",
			str:         "false",
			defValue:    true,
			expectedVal: false,
		},
		{
			name:        "Empty string",
			str:         "",
			defValue:    false,
			expectedVal: false,
		},
		{
			name:        "Invalid str",
			str:         "AAA",
			defValue:    true,
			expectedVal: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			res := GetBool(test.str, test.defValue)
			require.Equal(t, test.expectedVal, res)
		})
	}
}

func TestParseConfig(t *testing.T) {
	// Setup Logger
	logger := logrus.New()
	formatter := new(logrus.TextFormatter)
	formatter.TimestampFormat = time.RFC3339Nano
	formatter.FullTimestamp = true
	logger.SetFormatter(formatter)
	logger.SetLevel(logrus.DebugLevel)

	tests := []struct {
		name     string
		vc       string
		user     string
		password string
	}{
		{
			name:     `Password with special character \ in it`,
			vc:       "sc-rdops-vm06-dhcp-184-231.eng.vmware.com",
			user:     "Administrator@vsphere.local",
			password: `6^54#,RDvwgJ\Edg$2`,
		},
	}
	confData := `[Global]
	cluster-id = "cluster1"
	
	[VirtualCenter "sc-rdops-vm06-dhcp-184-231.eng.vmware.com"]
	user = "Administrator@vsphere.local"
	password = "6^54#,RDvwgJ\\Edg$2"
	port = "443"`
	mockSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "velero-vsphere-config-secret"},
		Data:       map[string][]byte{"csi-vsphere.conf": []byte(confData)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			params := make(map[string]interface{})
			ParseConfig(mockSecret, params, logger)
			assert.Equal(t, test.vc, params["VirtualCenter"])
			assert.Equal(t, test.password, params["password"])
		})
	}
}

func TestDeleteSvcSnapshot(t *testing.T) {
	tests := []struct {
		name                     string
		gcSnapshot               *backupdriverapi.Snapshot
		svcSnapshot              *backupdriverapi.Snapshot
		config                   *rest.Config
		toDeleteSvcSnapshotFirst bool
		expectedErr              bool
	}{
		{
			name: "If svcSnapshot has already been deleted, should not return error",
			gcSnapshot: &backupdriverapi.Snapshot{
				TypeMeta: metav1.TypeMeta{
					APIVersion: backupdriverapi.SchemeGroupVersion.String(),
					Kind:       "Snapshot",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Name:      "gc-snapshot-1",
				},
				Status: backupdriverapi.SnapshotStatus{
					SvcSnapshotName: "svc-snapshot-1",
				},
			},
			svcSnapshot: &backupdriverapi.Snapshot{
				TypeMeta: metav1.TypeMeta{
					APIVersion: backupdriverapi.SchemeGroupVersion.String(),
					Kind:       "Snapshot",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Name:      "svc-snapshot-1",
				},
			},
			config:                   &rest.Config{},
			toDeleteSvcSnapshotFirst: true,
			expectedErr:              false,
		},
		{
			name: "Delete a corresponding svc snapshot from gc snapshot",
			gcSnapshot: &backupdriverapi.Snapshot{
				TypeMeta: metav1.TypeMeta{
					APIVersion: backupdriverapi.SchemeGroupVersion.String(),
					Kind:       "Snapshot",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Name:      "gc-snapshot-2",
				},
				Status: backupdriverapi.SnapshotStatus{
					SvcSnapshotName: "svc-snapshot-2",
				},
			},
			svcSnapshot: &backupdriverapi.Snapshot{
				TypeMeta: metav1.TypeMeta{
					APIVersion: backupdriverapi.SchemeGroupVersion.String(),
					Kind:       "Snapshot",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Name:      "svc-snapshot-2",
				},
			},
			config:                   &rest.Config{},
			toDeleteSvcSnapshotFirst: false,
			expectedErr:              false,
		},
		{
			name: "Guest cluster with no svccnapshot name should return error",
			gcSnapshot: &backupdriverapi.Snapshot{
				TypeMeta: metav1.TypeMeta{
					APIVersion: backupdriverapi.SchemeGroupVersion.String(),
					Kind:       "Snapshot",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Name:      "gc-snapshot-3",
				},
			},
			svcSnapshot: &backupdriverapi.Snapshot{
				TypeMeta: metav1.TypeMeta{
					APIVersion: backupdriverapi.SchemeGroupVersion.String(),
					Kind:       "Snapshot",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Name:      "svc-snapshot-3",
				},
			},
			config:                   &rest.Config{},
			toDeleteSvcSnapshotFirst: false,
			expectedErr:              true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var (
				client             = fake.NewSimpleClientset(test.svcSnapshot)
				sharedInformers    = informers.NewSharedInformerFactory(client, 0)
				logger             = veleroplugintest.NewLogger()
				backupdriverClient = client.BackupdriverV1alpha1()
			)
			require.NoError(t, sharedInformers.Backupdriver().V1alpha1().Snapshots().Informer().GetStore().Add(test.gcSnapshot))
			if !test.toDeleteSvcSnapshotFirst {
				require.NoError(t, sharedInformers.Backupdriver().V1alpha1().Snapshots().Informer().GetStore().Add(test.svcSnapshot))
			}

			patches := gomonkey.ApplyFunc(GetBackupdriverClient, func(_ *rest.Config) (backupdriverTypedV1.BackupdriverV1alpha1Interface, error) {
				return backupdriverClient, nil
			})
			defer patches.Reset()
			patches.ApplyFunc(GetSupervisorConfig, func(_ *rest.Config, _ logrus.FieldLogger) (*rest.Config, string, error) {
				return &rest.Config{}, test.svcSnapshot.Namespace, nil
			})
			result := DeleteSvcSnapshot(test.gcSnapshot.Status.SvcSnapshotName, test.gcSnapshot.Namespace, test.gcSnapshot.Name, test.config, logger)
			if test.expectedErr {
				require.NotNil(t, result)
			} else {
				require.Nil(t, result)
			}
		})
	}
}

func TestGetComponentFromImage(t *testing.T) {
	tests := []struct {
		name                                             string
		image                                            string
		expectedRepo, expectedContainer, expectedVersion string
	}{
		{
			name:              "ExpectedDummyCase",
			image:             "a/b/c/d:x",
			expectedRepo:      "a/b/c",
			expectedContainer: "d",
			expectedVersion:   "x",
		},
		{
			name:              "ExpectedDockerhubCase",
			image:             "vsphereveleroplugin/velero-plugin-for-vsphere:1.1.0-rc2",
			expectedRepo:      "vsphereveleroplugin",
			expectedContainer: "velero-plugin-for-vsphere",
			expectedVersion:   "1.1.0-rc2",
		},
		{
			name:              "ExpectedCustomizedCase",
			image:             "xyz-repo.vmware.com/velero/velero-plugin-for-vsphere:1.1.0-rc2",
			expectedRepo:      "xyz-repo.vmware.com/velero",
			expectedContainer: "velero-plugin-for-vsphere",
			expectedVersion:   "1.1.0-rc2",
		},
		{
			name:              "ExpectedLocalCase",
			image:             "velero-plugin-for-vsphere:1.1.0-rc2",
			expectedRepo:      "",
			expectedContainer: "velero-plugin-for-vsphere",
			expectedVersion:   "1.1.0-rc2",
		},
		{
			name:              "ExpectedNonTaggedImageCase",
			image:             "xyz-repo.vmware.com/velero/velero-plugin-for-vsphere",
			expectedRepo:      "xyz-repo.vmware.com/velero",
			expectedContainer: "velero-plugin-for-vsphere",
			expectedVersion:   "",
		},
		{
			name:              "ExpectedCaseOfRegistryEndpointWithPort",
			image:             "xyz-repo.vmware.com:9999/velero/velero-plugin-for-vsphere:1.1.0-rc2",
			expectedRepo:      "xyz-repo.vmware.com:9999/velero",
			expectedContainer: "velero-plugin-for-vsphere",
			expectedVersion:   "1.1.0-rc2",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actualRepo := GetComponentFromImage(test.image, constants.ImageRepositoryComponent)
			actualContainer := GetComponentFromImage(test.image, constants.ImageContainerComponent)
			actualVersion := GetComponentFromImage(test.image, constants.ImageVersionComponent)
			assert.Equal(t, actualRepo, test.expectedRepo)
			assert.Equal(t, actualContainer, test.expectedContainer)
			assert.Equal(t, actualVersion, test.expectedVersion)
		})
	}
}

func TestGetS3SessionOptionsFromParamsMap(t *testing.T) {
	params1 := make(map[string]interface{})
	params1["region"] = "us-west-1"
	options1 := session.Options{Config: aws.Config{
		Region: aws.String("us-wes-1"),
	}}
	params2 := make(map[string]interface{})
	params2["region"] = "us-west-1"
	params2[constants.AWS_ACCESS_KEY_ID] = "key-id"
	params2[constants.AWS_SECRET_ACCESS_KEY] = "secret-access-key"
	options2 := session.Options{Config: aws.Config{
		Region:      aws.String("us-wes-1"),
		Credentials: credentials.NewStaticCredentials("key-id", "secret-access-key", ""),
	}}
	params3 := make(map[string]interface{})
	params3["region"] = "us-west-1"
	params3["caCert"] = "caCert"
	options3 := session.Options{Config: aws.Config{
		Region: aws.String("us-wes-1"),
	}}
	options3.CustomCABundle = strings.NewReader("caCert")
	tests := []struct {
		name     string
		params   map[string]interface{}
		expected session.Options
	}{
		{
			name:     "If the credentials are not explicitly provided in params. No caCert is provided.",
			params:   params1,
			expected: options1,
		},
		{
			name:     "If the credentials are explicitly provided in params. No caCert is provided.",
			params:   params2,
			expected: options2,
		},
		{
			name:     "If caCert is provided.",
			params:   params3,
			expected: options3,
		},
	}
	logger := veleroplugintest.NewLogger()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sessionOptions, err := GetS3SessionOptionsFromParamsMap(test.params, logger)
			assert.Nil(t, err)
			assert.Equal(t, test.params["region"], *sessionOptions.Config.Region)

			_, ok := test.params[constants.AWS_ACCESS_KEY_ID]
			if ok {
				assert.NotNil(t, sessionOptions.Config.Credentials)
			}

			_, ok = test.params["caCert"]
			if ok {
				assert.NotNil(t, sessionOptions.CustomCABundle)
			}
		})
	}
}

func TestGetVersionFromImage(t *testing.T) {
	tests := []struct {
		name       string
		key        string
		containers []corev1.Container
		expected   string
	}{
		{
			name: "Valid image string should return non-empty version",
			key:  "cloud-provider-vsphere/csi/release/driver",
			containers: []corev1.Container{
				{
					Image: "gcr.io/cloud-provider-vsphere/csi/release/driver:corev1.0.1",
				},
			},
			expected: "corev1.0.1",
		},
		{
			name: "Valid image string should return non-empty version",
			key:  "cloud-provider-vsphere/csi/release/driver",
			containers: []corev1.Container{
				{
					Image: "cloud-provider-vsphere/csi/release/driver:v2.0.0",
				},
			},
			expected: "v2.0.0",
		},
		{
			name: "Valid image string should return non-empty version",
			key:  "cloud-provider-vsphere/csi/release/driver",
			containers: []corev1.Container{
				{
					Image: "myregistry/cloud-provider-vsphere/csi/release/driver:v2.0.0",
				},
			},
			expected: "v2.0.0",
		},
		{
			name: "Valid image string should return non-empty version",
			key:  "cloud-provider-vsphere/csi/release/driver",
			containers: []corev1.Container{
				{
					Image: "myregistry/level1/level2/cloud-provider-vsphere/csi/release/driver:v2.0.0",
				},
			},
			expected: "v2.0.0",
		},
		{
			name: "Invalid image name should return empty string",
			key:  "cloud-provider-vsphere/csi/release/driver",
			containers: []corev1.Container{
				{
					Image: "gcr.io/csi/release/driver:corev1.0.1",
				},
			},
			expected: "",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			version := GetVersionFromImage(test.containers, test.key)
			assert.Equal(t, test.expected, version)
		})
	}
}

func TestGetCSIClusterType(t *testing.T) {
	tests := []struct {
		name                string
		runtimeObjs         []runtime.Object
		expectedError       error
		expectedClusterType constants.ClusterFlavor
	}{
		{
			name: "CSI v1.0.2 Vanilla Deployment",
			runtimeObjs: []runtime.Object{
				csi102StatefultSet,
			},
			expectedClusterType: constants.VSphere,
			expectedError:       nil,
		},
		{
			name: "CSI v1.0.2 Vanilla Deployment with no vsphere-csi-controller container image",
			runtimeObjs: []runtime.Object{
				&appsv1.StatefulSet{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "kube-system",
						Name:      "vsphere-csi-controller",
					},
					Spec: appsv1.StatefulSetSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "not-a-vsphere-csi-controller",
										Image: "xyz.io:9999/cloud-provider-vsphere/csi/release/driver:v1.0.3",
									},
								},
							},
						},
					},
				},
			},
			expectedClusterType: constants.Unknown,
			expectedError:       errors.New("Expected CSI driver container images not found while inferring cluster type."),
		},
		{
			name: "CSI v2.0.1 Vanilla Deployment",
			runtimeObjs: []runtime.Object{
				csi201VanillaDeployment,
			},
			expectedClusterType: constants.VSphere,
			expectedError:       nil,
		},
		{
			name: "CSI v2.3.0 Vanilla Deployment",
			runtimeObjs: []runtime.Object{
				csi230VanillaDeployment,
			},
			expectedClusterType: constants.VSphere,
			expectedError:       nil,
		},
		{
			name: "CSI 2.3.0 Deployment with no vsphere-csi-controller container image",
			runtimeObjs: []runtime.Object{
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "vmware-system-csi",
						Name:      "vsphere-csi-controller",
					},
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "not-a-vsphere-csi-controller",
										Image: "gcr.io/cloud-provider-vsphere/csi/release/driver:v2.0.1",
									},
								},
							},
						},
					},
				},
			},
			expectedClusterType: constants.Unknown,
			expectedError:       errors.New("Expected CSI driver container images not found while inferring cluster type."),
		},
		{
			name:                "CSI Driver is not deployed",
			runtimeObjs:         []runtime.Object{},
			expectedClusterType: constants.Unknown,
			expectedError:       errors.New("vSphere CSI controller, vsphere-csi-controller, is required by velero-plugin-for-vsphere. Please make sure the vSphere CSI controller is installed in the cluster"),
		},
		{
			name: "CSI Supervisor Deployment",
			runtimeObjs: []runtime.Object{
				csi230SupervisorDeployment,
			},
			expectedClusterType: constants.Supervisor,
			expectedError:       nil,
		},
		{
			name: "CSI Guest Deployment",
			runtimeObjs: []runtime.Object{
				csi230GuestDeployment,
			},
			expectedClusterType: constants.TkgGuest,
			expectedError:       nil,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			kubeClient := kubeclientfake.NewSimpleClientset(test.runtimeObjs...)
			actualClusterType, actualError := GetCSIClusterType(kubeClient)
			if test.expectedError == nil {
				assert.Equal(t, test.expectedClusterType, actualClusterType)
			} else {
				// expected an error, but no error was thrown.
				assert.NotNil(t, actualError, "No error thrown")
				// Ensure errors match
				assert.Equal(t, test.expectedError.Error(), actualError.Error())
			}
		})
	}
}

func TestGetClusterTypeFromConfig(t *testing.T) {
	tests := []struct {
		name                string
		runtimeObjs         []runtime.Object
		expectedError       error
		expectedClusterType constants.ClusterFlavor
	}{
		{
			name: "VANILLA specified in velero-vsphere-plugin-config",
			runtimeObjs: []runtime.Object{
				pluginConfigVanilla,
			},
			expectedClusterType: constants.VSphere,
			expectedError:       nil,
		},
		{
			name: "GUEST specified in velero-vsphere-plugin-config",
			runtimeObjs: []runtime.Object{
				pluginConfigGuest,
			},
			expectedClusterType: constants.TkgGuest,
			expectedError:       nil,
		},
		{
			name: "INVALID specified in velero-vsphere-plugin-config",
			runtimeObjs: []runtime.Object{
				pluginConfigInvalid,
			},
			expectedClusterType: constants.Unknown,
			expectedError:       nil,
		},
		{
			name:                "No velero-vsphere-plugin-config ConfigMap in the cluster",
			expectedClusterType: constants.Unknown,
			expectedError:       errors.New("Not Found error"),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			kubeClient := kubeclientfake.NewSimpleClientset(test.runtimeObjs...)
			actualClusterType, actualError := GetClusterTypeFromConfig(kubeClient, constants.DefaultVeleroNamespace,
				constants.VeleroVSpherePluginConfig)
			if test.expectedError == nil {
				assert.Equal(t, test.expectedClusterType, actualClusterType)
			} else {
				// expected an error, but no error was thrown.
				assert.NotNil(t, actualError, "No error thrown")
				// ensure the cluster type is unknown.
				assert.Equal(t, constants.Unknown, actualClusterType)
			}
		})
	}
}

func TestRetrieveClusterFlavor(t *testing.T) {
	tests := []struct {
		name                string
		runtimeObjs         []runtime.Object
		expectedError       error
		expectedClusterType constants.ClusterFlavor
	}{
		{
			name: "Config velero-vsphere-plugin-config present with VANILLA cluster flavor specified",
			runtimeObjs: []runtime.Object{
				pluginConfigVanilla,
			},
			expectedClusterType: constants.VSphere,
			expectedError:       nil,
		},
		{
			name: "Test fallback when Config velero-vsphere-plugin-config present with cluster_flavor INVALID and csi 2.3 Deployment on Vanilla",
			runtimeObjs: []runtime.Object{
				pluginConfigInvalid,
				csi230VanillaDeployment,
			},
			expectedClusterType: constants.VSphere,
			expectedError:       nil,
		},
		{
			name: "Test fallback when Config velero-vsphere-plugin-config is absent and csi 2.3 Deployment",
			runtimeObjs: []runtime.Object{
				csi230VanillaDeployment,
			},
			expectedClusterType: constants.VSphere,
			expectedError:       nil,
		},
		{
			name: "Test fallback when Config velero-vsphere-plugin-config is absent and csi 2.0.1 Deployment",
			runtimeObjs: []runtime.Object{
				csi201VanillaDeployment,
			},
			expectedClusterType: constants.VSphere,
			expectedError:       nil,
		},
		{
			name:                "Test fallback when Config velero-vsphere-plugin-config is absent and csi driver is absent",
			runtimeObjs:         []runtime.Object{},
			expectedClusterType: constants.VSphere,
			expectedError:       nil,
		},
		{
			name: "Test fallback when Config velero-vsphere-plugin-config is absent and csi 2.3 Deployment on Supervisor",
			runtimeObjs: []runtime.Object{
				csi230SupervisorDeployment,
			},
			expectedClusterType: constants.Supervisor,
			expectedError:       nil,
		},
		{
			name: "Test fallback when Config velero-vsphere-plugin-config is absent and csi 2.3 Deployment on Guest",
			runtimeObjs: []runtime.Object{
				csi230GuestDeployment,
			},
			expectedClusterType: constants.TkgGuest,
			expectedError:       nil,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			kubeClient := kubeclientfake.NewSimpleClientset(test.runtimeObjs...)
			actualClusterType, actualError := retrieveClusterFlavor(kubeClient, constants.DefaultVeleroNamespace)
			if test.expectedError == nil {
				assert.Equal(t, test.expectedClusterType, actualClusterType)
			} else {
				// expected an error, but no error was thrown.
				assert.NotNil(t, actualError, "No error thrown")
				// ensure the cluster type is unknown.
				assert.Equal(t, constants.Unknown, actualClusterType)
			}
		})
	}
}

func TestRetrieveVcConfigSecret(t *testing.T) {
	// Setup Logger
	logger := logrus.New()
	formatter := new(logrus.TextFormatter)
	formatter.TimestampFormat = time.RFC3339Nano
	formatter.FullTimestamp = true
	logger.SetFormatter(formatter)
	logger.SetLevel(logrus.DebugLevel)
	confData := `[Global]
	cluster-id = "cluster1"
	
	[VirtualCenter "10.182.1.133"]
	user = "user@vsphere.local"
	password = "password"
	port = "443"`
	mockSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "velero-vsphere-config-secret"},
		Data:       map[string][]byte{"csi-vsphere.conf": []byte(confData)},
	}
	tests := []struct {
		name        string
		runtimeObjs []runtime.Object
		config      *rest.Config
		expectError bool
	}{
		{
			name: "Test secret retrieval",
			runtimeObjs: []runtime.Object{
				decoupleVSphereCSIDriverFeatureDisabled,
				csi201VanillaDeployment,
				csi201VSphereCredentialSecret,
				mockSecret,
			},
			config:      &rest.Config{},
			expectError: false,
		},
		{
			name: "Decouple CSI driver feature enabled, plugin config absent, supervisor csi 2.3, csi secret present",
			runtimeObjs: []runtime.Object{
				decoupleVSphereCSIDriverFeatureEnabled,
				csi230SupervisorDeployment,
				csi230VSphereCredentialSecret,
			},
			config:      &rest.Config{},
			expectError: false,
		},
		{
			name: "Decouple CSI driver feature enabled, plugin config disabled, supervisor csi 2.3, csi secret present",
			runtimeObjs: []runtime.Object{
				decoupleVSphereCSIDriverFeatureDisabled,
				csi230SupervisorDeployment,
				csi230VSphereCredentialSecret,
			},
			config:      &rest.Config{},
			expectError: false,
		},
		{
			name: "Decouple CSI driver feature enabled, plugin config present but no secret specified, plugin secret present",
			runtimeObjs: []runtime.Object{
				pluginConfigVanillaSecretUnspecified,
				decoupleVSphereCSIDriverFeatureEnabled,
				pluginVSphereCredentialSecret,
			},
			config:      &rest.Config{},
			expectError: false,
		},
		{
			name: "Decouple CSI driver feature enabled, plugin config present, plugin secret present",
			runtimeObjs: []runtime.Object{
				pluginConfigVanilla,
				decoupleVSphereCSIDriverFeatureEnabled,
				pluginVSphereCredentialSecret,
			},
			config:      &rest.Config{},
			expectError: false,
		},
		{
			name: "Decouple CSI driver feature enabled, plugin config absent, default plugin secret present",
			runtimeObjs: []runtime.Object{
				decoupleVSphereCSIDriverFeatureEnabled,
				pluginVSphereCredentialSecret,
			},
			config:      &rest.Config{},
			expectError: false,
		},
		{
			name: "Decouple CSI driver feature enabled, plugin config absent, default plugin secret present",
			runtimeObjs: []runtime.Object{
				decoupleVSphereCSIDriverFeatureEnabled,
				pluginVSphereCredentialSecret,
			},
			config:      &rest.Config{},
			expectError: false,
		},
		{
			name: "Decouple CSI driver feature enabled, plugin config absent, default plugin secret absent",
			runtimeObjs: []runtime.Object{
				decoupleVSphereCSIDriverFeatureEnabled,
			},
			config:      &rest.Config{},
			expectError: true,
		},
		{
			name: "Decouple CSI driver feature disabled, vanilla csi 2.3, csi secret present",
			runtimeObjs: []runtime.Object{
				decoupleVSphereCSIDriverFeatureDisabled,
				csi230VanillaDeployment,
				csi230VSphereCredentialSecret,
			},
			config:      &rest.Config{},
			expectError: false,
		},
		{
			name: "Decouple CSI driver feature disabled, vanilla csi 2.3, csi secret absent",
			runtimeObjs: []runtime.Object{
				decoupleVSphereCSIDriverFeatureDisabled,
				csi230VanillaDeployment,
			},
			config:      &rest.Config{},
			expectError: true,
		},
		{
			name: "Decouple CSI driver feature disabled, vanilla csi 2.0.1, csi secret present",
			runtimeObjs: []runtime.Object{
				decoupleVSphereCSIDriverFeatureDisabled,
				csi201VanillaDeployment,
				csi201VSphereCredentialSecret,
			},
			config:      &rest.Config{},
			expectError: false,
		},
		{
			name: "Decouple CSI driver feature disabled, vanilla csi 2.0.1, csi secret present",
			runtimeObjs: []runtime.Object{
				decoupleVSphereCSIDriverFeatureDisabled,
				csi201VanillaDeployment,
				csi201VSphereCredentialSecret,
			},
			config:      &rest.Config{},
			expectError: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			kubeClient := kubeclientfake.NewSimpleClientset(test.runtimeObjs...)
			params := make(map[string]interface{})
			patches := gomonkey.ApplyFunc(GetKubeClientSet, func(_ *rest.Config) (kubernetes.Interface, error) {
				return kubeClient, nil
			})
			defer patches.Reset()
			patches.ApplyFunc(GetVeleroNamespace, func() (string, bool) {
				return constants.DefaultVeleroNamespace, true
			})
			retErr := RetrieveVcConfigSecret(params, test.config, logger)
			if retErr != nil {
				// Check if the test expected error if not fail.
				if !test.expectError == true {
					t.Fatalf("Unexpected error received.\n test: %s\n err: %+v", test.name, retErr)
				}
			} else {
				// Check if the test expected error if not fail.
				if test.expectError == true {
					t.Fatalf("Expected error in scenario, but did not recieve it.\n test: %s", test.name)
				}
			}
		})
	}
}

func TestGetUploadCRRetryMaximumFromConfig(t *testing.T) {
	tests := []struct {
		name                     string
		runtimeObjs              []runtime.Object
		expectedError            error
		expectedUploadCRRetryMax int
	}{
		{
			name: "upload-cr-retry-max is not specified in velero-vsphere-plugin-config",
			runtimeObjs: []runtime.Object{
				pluginConfigVanilla,
			},
			expectedUploadCRRetryMax: constants.DefaultUploadCRRetryMaximum,
			expectedError:            nil,
		},
		{
			name: "upload-cr-retry-max is specified in velero-vsphere-plugin-config",
			runtimeObjs: []runtime.Object{
				pluginConfigVanillaWithUploadCRRetryMax,
			},
			expectedUploadCRRetryMax: 5,
			expectedError:            nil,
		},
	}

	logger := veleroplugintest.NewLogger()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			kubeClient := kubeclientfake.NewSimpleClientset(test.runtimeObjs...)
			actualUploadCRRetryMax := GetUploadCRRetryMaximumFromConfig(kubeClient, constants.DefaultVeleroNamespace,
				constants.VeleroVSpherePluginConfig, logger)
			assert.Equal(t, test.expectedUploadCRRetryMax, actualUploadCRRetryMax)
		})
	}
}
