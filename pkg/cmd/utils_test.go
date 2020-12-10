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

package cmd

import (
	"context"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/constants"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/install"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubeclientfake "k8s.io/client-go/kubernetes/fake"
	"strconv"
	"testing"
)

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

func TestGetCompatibleRepoAndTagFromPluginImage(t *testing.T) {
	tests := []struct {
		name             string
		veleroDeployment *appsv1.Deployment
		targetContainer  string
		expectedImage    string
		expectedError    error
	}{
		{
			name: "ExpectedPluginImageFromDockerhub",
			veleroDeployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "velero",
					Name:      constants.VeleroDeployment,
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							InitContainers: []corev1.Container{
								{
									Name:  "velero-plugin-for-vsphere",
									Image: "vsphereveleroplugin/velero-plugin-for-vsphere:1.1.0-rc2",
								},
							},
						},
					},
				},
			},
			targetContainer: constants.BackupDriverForPlugin,
			expectedImage:   "vsphereveleroplugin/" + constants.BackupDriverForPlugin + ":1.1.0-rc2",
			expectedError:   nil,
		},
		{
			name: "UnexpectedDeployment",
			veleroDeployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "xyz",
					Name:      "not-velero",
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							InitContainers: []corev1.Container{
								{
									Name:  "velero-plugin-for-vsphere",
									Image: "vsphereveleroplugin/velero-plugin-for-vsphere:1.1.0-rc2",
								},
							},
						},
					},
				},
			},
			targetContainer: constants.BackupDriverForPlugin,
			expectedImage:   "",
			expectedError:   errors.Errorf("Failed to get velero deployment in namespace %s", "xyz"),
		},
		{
			name: "NoExpectedPluginImageAvailable",
			veleroDeployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "velero",
					Name:      constants.VeleroDeployment,
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							InitContainers: []corev1.Container{
								{
									Name:  "velero-plugin-for-aws",
									Image: "velero/velero-plugin-for-aws:1.1.0",
								},
							},
						},
					},
				},
			},
			targetContainer: constants.BackupDriverForPlugin,
			expectedImage:   "",
			expectedError:   errors.New("The plugin, velero-plugin-for-vsphere, was not added as an init container of Velero deployment"),
		},
		{
			name: "ExpectedPluginImageFromOnPremRegistry",
			veleroDeployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "velero",
					Name:      constants.VeleroDeployment,
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							InitContainers: []corev1.Container{
								{
									Name:  "velero-plugin-for-vsphere",
									Image: "xyz-repo.opq.abc:8888/one/two/velero-plugin-for-vsphere:1.1.0-rc2",
								},
							},
						},
					},
				},
			},
			targetContainer: constants.BackupDriverForPlugin,
			expectedImage:   "xyz-repo.opq.abc:8888/one/two/" + constants.BackupDriverForPlugin + ":1.1.0-rc2",
			expectedError:   nil,
		},
		{
			name: "ExpectedPluginImageWithoutTag",
			veleroDeployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "velero",
					Name:      constants.VeleroDeployment,
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							InitContainers: []corev1.Container{
								{
									Name:  "velero-plugin-for-vsphere",
									Image: "xyz-repo.opq.abc:8888/one/two/velero-plugin-for-vsphere",
								},
							},
						},
					},
				},
			},
			targetContainer: constants.DataManagerForPlugin,
			expectedImage:   "xyz-repo.opq.abc:8888/one/two/" + constants.DataManagerForPlugin,
			expectedError:   nil,
		},
		{
			name: "ExpectedPluginImageWithoutRepo",
			veleroDeployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "velero",
					Name:      constants.VeleroDeployment,
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							InitContainers: []corev1.Container{
								{
									Name:  "velero-plugin-for-vsphere",
									Image: "velero-plugin-for-vsphere:1.1.0-rc2",
								},
							},
						},
					},
				},
			},
			targetContainer: constants.DataManagerForPlugin,
			expectedImage:   constants.DataManagerForPlugin + ":1.1.0-rc2",
			expectedError:   nil,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			kubeClient := kubeclientfake.NewSimpleClientset(test.veleroDeployment)
			actualImage, actualError := GetCompatibleRepoAndTagFromPluginImage(kubeClient, test.veleroDeployment.Namespace, test.targetContainer)
			assert.Equal(t, test.expectedImage, actualImage)
			assert.Equal(t, test.expectedError == nil, actualError == nil)
			if actualError != nil {
				assert.Equal(t, test.expectedError.Error(), actualError.Error())
			}
		})
	}
}

func TestGetVeleroFeatureFlags(t *testing.T) {
	tests := []struct {
		name                 string
		veleroDeployment     *appsv1.Deployment
		expectedFeatureFlags []string
		expectedError        error
	}{
		{
			name: "ExpectedSingleFeatureFlag",
			veleroDeployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "velero",
					Name:      constants.VeleroDeployment,
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "velero",
									Image: "velero/velero:v1.5.1",
									Args: []string{
										"server",
										"--features=EnableVSphereItemActionPlugin",
									},
								},
							},
						},
					},
				},
			},
			expectedFeatureFlags: []string{"EnableVSphereItemActionPlugin"},
			expectedError:        nil,
		},
		{
			name: "ExpectedMultipleFeatureFlags",
			veleroDeployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "velero",
					Name:      constants.VeleroDeployment,
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "velero",
									Image: "velero/velero:v1.5.1",
									Args: []string{
										"server",
										"--features=EnableVSphereItemActionPlugin,EnableLocalMode",
									},
								},
							},
						},
					},
				},
			},
			expectedFeatureFlags: []string{"EnableVSphereItemActionPlugin", "EnableLocalMode"},
			expectedError:        nil,
		},
		{
			name: "ExpectedMultipleFeatureFlagsWithOtherArgs",
			veleroDeployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "velero",
					Name:      constants.VeleroDeployment,
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "velero",
									Image: "velero/velero:v1.5.1",
									Args: []string{
										"server",
										"--features=EnableVSphereItemActionPlugin,EnableLocalMode",
										"--metrics-address=:0",
									},
								},
							},
						},
					},
				},
			},
			expectedFeatureFlags: []string{"EnableVSphereItemActionPlugin", "EnableLocalMode"},
			expectedError:        nil,
		},
		{
			name: "ExpectedMultipleFeatureFlagsFromUnexpectedContainer",
			veleroDeployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "velero",
					Name:      constants.VeleroDeployment,
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "non-velero",
									Image: "velero/velero:v1.5.1",
									Args: []string{
										"server",
										"--features=EnableVSphereItemActionPlugin,EnableLocalMode",
										"--metrics-address=:0",
									},
								},
							},
						},
					},
				},
			},
			expectedFeatureFlags: []string{},
			expectedError:        nil,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			kubeClient := kubeclientfake.NewSimpleClientset(test.veleroDeployment)
			actualFeatureFlags, actualError := GetVeleroFeatureFlags(kubeClient, test.veleroDeployment.Namespace)
			assert.Equal(t, test.expectedError == nil, actualError == nil)
			assert.Equal(t, test.expectedFeatureFlags, actualFeatureFlags)
		})
	}
}

func TestCreateFeatureStateConfigMap(t *testing.T) {
	tests := []struct {
		name                    string
		featureConfigMap        *corev1.ConfigMap
		features                []string
		namespace               string
		expectedLocalModeStatus bool
		expectedError           error
	}{
		{
			name:                    "ExpectedCreateConfigMapCase",
			featureConfigMap:        &corev1.ConfigMap{},
			features:                []string{"EnableVSphereItemActionPlugin", constants.VSphereLocalModeFeature},
			namespace:               "velero",
			expectedLocalModeStatus: true,
			expectedError:           nil,
		},
		{
			name:                    "ExpectedCreateConfigMapCaseTurnOff",
			featureConfigMap:        &corev1.ConfigMap{},
			features:                []string{"EnableVSphereItemActionPlugin"},
			namespace:               "velero",
			expectedLocalModeStatus: false,
			expectedError:           nil,
		},
		{
			name: "ExpectedUpdateConfigMapCase",
			featureConfigMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.VSpherePluginFeatureStates,
					Namespace: "velero",
				},
				Data: map[string]string{
					constants.VSphereLocalModeFlag: "false",
				},
			},
			features:                []string{"EnableVSphereItemActionPlugin", constants.VSphereLocalModeFeature},
			namespace:               "velero",
			expectedLocalModeStatus: true,
			expectedError:           nil,
		},
		{
			name: "ExpectedUpdateConfigMapCaseTurnOff",
			featureConfigMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      constants.VSpherePluginFeatureStates,
					Namespace: "velero",
				},
				Data: map[string]string{
					constants.VSphereLocalModeFlag: "true",
				},
			},
			features:                []string{"EnableVSphereItemActionPlugin"},
			namespace:               "velero",
			expectedLocalModeStatus: false,
			expectedError:           nil,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			kubeClient := kubeclientfake.NewSimpleClientset(test.featureConfigMap)
			actualError := CreateFeatureStateConfigMap(kubeClient, test.features, test.namespace)
			assert.Equal(t, test.expectedError == nil, actualError == nil)
			featureConfigMap, _ := kubeClient.CoreV1().ConfigMaps(test.namespace).Get(context.Background(), constants.VSpherePluginFeatureStates, metav1.GetOptions{})
			assert.NotNil(t, featureConfigMap)
			assert.Equal(t, strconv.FormatBool(test.expectedLocalModeStatus), featureConfigMap.Data[constants.VSphereLocalModeFlag])
		})
	}
}

func TestCheckPluginImageRepo(t *testing.T) {
	tests := []struct {
		name             string
		veleroDeployment *appsv1.Deployment
		defaultImage     string
		serverType       string
		expectedImage    string
		expectedError    error
	}{
		{
			name: "ExpectedOfficialPluginImage",
			veleroDeployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "velero",
					Name:      constants.VeleroDeployment,
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							InitContainers: []corev1.Container{
								{
									Name:  "velero-plugin-for-vsphere",
									Image: "vsphereveleroplugin/velero-plugin-for-vsphere:1.1.0-rc2",
								},
							},
						},
					},
				},
			},
			defaultImage:  install.DefaultBackupDriverImage,
			serverType:    constants.BackupDriverForPlugin,
			expectedImage: "vsphereveleroplugin/" + constants.BackupDriverForPlugin + ":1.1.0-rc2",
			expectedError: nil,
		},
		{
			name: "ExpectedCustomizedPluginImage",
			veleroDeployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "velero",
					Name:      constants.VeleroDeployment,
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							InitContainers: []corev1.Container{
								{
									Name:  "velero-plugin-for-vsphere",
									Image: "xyz.io:9999/one/two/velero-plugin-for-vsphere:1.1.0-rc2",
								},
							},
						},
					},
				},
			},
			defaultImage:  install.DefaultBackupDriverImage,
			serverType:    constants.BackupDriverForPlugin,
			expectedImage: "xyz.io:9999/one/two/" + constants.BackupDriverForPlugin + ":1.1.0-rc2",
			expectedError: nil,
		},
		{
			name: "NoExpectedPluginImageAvailable",
			veleroDeployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "velero",
					Name:      constants.VeleroDeployment,
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							InitContainers: []corev1.Container{
								{
									Name:  "velero-plugin-for-aws",
									Image: "velero/velero-plugin-for-aws:1.1.0",
								},
							},
						},
					},
				},
			},
			defaultImage:  install.DefaultBackupDriverImage,
			serverType:    constants.BackupDriverForPlugin,
			expectedImage: install.DefaultBackupDriverImage,
			expectedError: errors.New("The plugin, velero-plugin-for-vsphere, was not added as an init container of Velero deployment"),
		},
		{
			name: "UnexpectedDeployment",
			veleroDeployment: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "xyz",
					Name:      "not-velero",
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							InitContainers: []corev1.Container{
								{
									Name:  "velero-plugin-for-vsphere",
									Image: "vsphereveleroplugin/velero-plugin-for-vsphere:1.1.0-rc2",
								},
							},
						},
					},
				},
			},
			defaultImage:  install.DefaultBackupDriverImage,
			serverType:    constants.BackupDriverForPlugin,
			expectedImage: install.DefaultBackupDriverImage,
			expectedError: errors.Errorf("Failed to get velero deployment in namespace %s", "xyz"),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			kubeClient := kubeclientfake.NewSimpleClientset(test.veleroDeployment)
			actualImage, actualError := CheckPluginImageRepo(kubeClient, test.veleroDeployment.Namespace, test.defaultImage, test.serverType)
			assert.Equal(t, test.expectedImage, actualImage)
			assert.Equal(t, test.expectedError == nil, actualError == nil)
			if actualError != nil {
				assert.Equal(t, test.expectedError.Error(), actualError.Error())
			}
		})
	}
}

func TestCheckVSphereCSIDriverVersion(t *testing.T) {
	tests := []struct {
		name          string
		runtimeObjs   []runtime.Object
		clusterFlavor constants.ClusterFlavor
		expectedError error
	}{
		{
			name: "Positive Case in CSI v2.0.1 Vanilla",
			runtimeObjs: []runtime.Object{
				&appsv1.Deployment{
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
									},
									{
										Name:  "vsphere-syncer",
										Image: "gcr.io/cloud-provider-vsphere/csi/release/syncer:v2.0.1",
									},
								},
							},
						},
					},
				},
			},
			clusterFlavor: constants.VSphere,
			expectedError: nil,
		},
		{
			name: "Positive Case in CSI v2.0.1 Vanilla with Customized Registry Endpoint",
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
										Name:  "vsphere-csi-controller",
										Image: "xyz.io:9999/cloud-provider-vsphere/csi/release/driver:v2.0.1",
									},
									{
										Name:  "vsphere-syncer",
										Image: "xyz.io:9999/cloud-provider-vsphere/csi/release/syncer:v2.0.1",
									},
								},
							},
						},
					},
				},
			},
			clusterFlavor: constants.VSphere,
			expectedError: nil,
		},
		{
			name: "Positive Case in CSI v2.1.0 dev Vanilla",
			runtimeObjs: []runtime.Object{
				&appsv1.Deployment{
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
										Image: "gcr.io/cloud-provider-vsphere/csi/ci/driver:latest",
									},
									{
										Name:  "vsphere-syncer",
										Image: "gcr.io/cloud-provider-vsphere/csi/ci/syncer:latest",
									},
								},
							},
						},
					},
				},
			},
			clusterFlavor: constants.VSphere,
			expectedError: nil,
		},
		{
			name: "Negative Case in CSI v2.0.1 Vanilla with Unexpected Images",
			runtimeObjs: []runtime.Object{
				&appsv1.Deployment{
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
										Image: "gcr.io/cloud-provider-vsphere/csi/xyz/driver:v2.0.1",
									},
									{
										Name:  "vsphere-syncer",
										Image: "gcr.io/cloud-provider-vsphere/csi/ci/syncer:v2.0.1",
									},
								},
							},
						},
					},
				},
			},
			clusterFlavor: constants.VSphere,
			expectedError: errors.New("Expected CSI driver/syncer images not found"),
		},
		{
			name: "Negative Case in CSI v2.0.1 Vanilla with Unexpected Deployment Name",
			runtimeObjs: []runtime.Object{
				&appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: "kube-system",
						Name:      "vsphere-csi-driver",
					},
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "vsphere-csi-controller",
										Image: "gcr.io/cloud-provider-vsphere/csi/release/driver:v2.0.1",
									},
									{
										Name:  "vsphere-syncer",
										Image: "gcr.io/cloud-provider-vsphere/csi/release/syncer:v2.0.1",
									},
								},
							},
						},
					},
				},
			},
			clusterFlavor: constants.VSphere,
			expectedError: errors.Errorf("vSphere CSI controller, %s, is required by velero-plugin-for-vsphere. Please make sure the vSphere CSI controller is installed in the cluster", constants.VSphereCSIController),
		},
		{
			name: "Negative Case in CSI v1.0.1 Vanilla with Unexpected Deployment Name",
			runtimeObjs: []runtime.Object{
				&appsv1.Deployment{
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
										Image: "gcr.io/cloud-provider-vsphere/csi/release/driver:v1.0.1",
									},
									{
										Name:  "vsphere-syncer",
										Image: "gcr.io/cloud-provider-vsphere/csi/release/syncer:v1.0.1",
									},
								},
							},
						},
					},
				},
			},
			clusterFlavor: constants.VSphere,
			expectedError: errors.Errorf("The version of vSphere CSI controller is below the minimum requirement (%s)", constants.CsiMinVersion),
		},
		{
			name: "Positive case in CSI driver Guest",
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
										Name:  "vsphere-csi-controller",
										Image: "abc.io/vsphere-csi:v0.0.1.alpha_abc.79-7ecdcb2",
									},
									{
										Name:  "vsphere-syncer",
										Image: "abc.io/syncer:v0.0.1.alpha_abc.79-7ecdcb2",
									},
								},
							},
						},
					},
				},
			},
			clusterFlavor: constants.TkgGuest,
			expectedError: nil,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			kubeClient := kubeclientfake.NewSimpleClientset(test.runtimeObjs...)
			actualError := CheckVSphereCSIDriverVersion(kubeClient, test.clusterFlavor)
			assert.Equal(t, test.expectedError == nil, actualError == nil)
			if actualError != nil {
				assert.Equal(t, test.expectedError.Error(), actualError.Error())
			}
		})
	}
}
