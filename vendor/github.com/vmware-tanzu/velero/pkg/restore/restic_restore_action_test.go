/*
Copyright 2019 the Velero contributors.

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

package restore

import (
	"fmt"
	"sort"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1api "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"

	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"github.com/vmware-tanzu/velero/pkg/builder"
	"github.com/vmware-tanzu/velero/pkg/buildinfo"
	velerofake "github.com/vmware-tanzu/velero/pkg/generated/clientset/versioned/fake"
	"github.com/vmware-tanzu/velero/pkg/plugin/velero"
	velerotest "github.com/vmware-tanzu/velero/pkg/test"
	"github.com/vmware-tanzu/velero/pkg/util/kube"
)

func TestGetImage(t *testing.T) {
	configMapWithData := func(key, val string) *corev1api.ConfigMap {
		return &corev1api.ConfigMap{
			Data: map[string]string{
				key: val,
			},
		}
	}

	originalVersion := buildinfo.Version
	buildinfo.Version = "buildinfo-version"
	defer func() {
		buildinfo.Version = originalVersion
	}()

	tests := []struct {
		name      string
		configMap *corev1api.ConfigMap
		want      string
	}{
		{
			name:      "nil config map returns default image with buildinfo.Version as tag",
			configMap: nil,
			want:      fmt.Sprintf("%s:%s", defaultImageBase, buildinfo.Version),
		},
		{
			name:      "config map without 'image' key returns default image with buildinfo.Version as tag",
			configMap: configMapWithData("non-matching-key", "val"),
			want:      fmt.Sprintf("%s:%s", defaultImageBase, buildinfo.Version),
		},
		{
			name:      "config map with invalid data in 'image' key returns default image with buildinfo.Version as tag",
			configMap: configMapWithData("image", "not:valid:image"),
			want:      fmt.Sprintf("%s:%s", defaultImageBase, buildinfo.Version),
		},
		{
			name:      "config map with untagged image returns image with buildinfo.Version as tag",
			configMap: configMapWithData("image", "myregistry.io/my-image"),
			want:      fmt.Sprintf("%s:%s", "myregistry.io/my-image", buildinfo.Version),
		},
		{
			name:      "config map with tagged image returns tagged image",
			configMap: configMapWithData("image", "myregistry.io/my-image:my-tag"),
			want:      "myregistry.io/my-image:my-tag",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, getImage(velerotest.NewLogger(), test.configMap))
		})
	}
}

// TestResticRestoreActionExecute tests the restic restore item action plugin's Execute method.
func TestResticRestoreActionExecute(t *testing.T) {
	resourceReqs, _ := kube.ParseResourceRequirements(
		defaultCPURequestLimit, defaultMemRequestLimit, // requests
		defaultCPURequestLimit, defaultMemRequestLimit, // limits
	)

	var (
		restoreName = "my-restore"
		backupName  = "test-backup"
		veleroNs    = "velero"
	)

	tests := []struct {
		name             string
		pod              *corev1api.Pod
		podVolumeBackups []*velerov1api.PodVolumeBackup
		want             *corev1api.Pod
	}{
		{
			name: "Restoring pod with no other initContainers adds the restic initContainer",
			pod: builder.ForPod("ns-1", "my-pod").ObjectMeta(
				builder.WithAnnotations("snapshot.velero.io/myvol", "")).
				Result(),
			want: builder.ForPod("ns-1", "my-pod").
				ObjectMeta(
					builder.WithAnnotations("snapshot.velero.io/myvol", "")).
				InitContainers(
					newResticInitContainerBuilder(initContainerImage(defaultImageBase), "").
						Resources(&resourceReqs).
						VolumeMounts(builder.ForVolumeMount("myvol", "/restores/myvol").Result()).Result()).
				Result(),
		},
		{
			name: "Restoring pod with other initContainers adds the restic initContainer as the first one",
			pod: builder.ForPod("ns-1", "my-pod").
				ObjectMeta(
					builder.WithAnnotations("snapshot.velero.io/myvol", "")).
				InitContainers(builder.ForContainer("first-container", "").Result()).
				Result(),
			want: builder.ForPod("ns-1", "my-pod").
				ObjectMeta(
					builder.WithAnnotations("snapshot.velero.io/myvol", "")).
				InitContainers(
					newResticInitContainerBuilder(initContainerImage(defaultImageBase), "").
						Resources(&resourceReqs).
						VolumeMounts(builder.ForVolumeMount("myvol", "/restores/myvol").Result()).Result(),
					builder.ForContainer("first-container", "").Result()).
				Result(),
		},
		{
			name: "Restoring pod with other initContainers adds the restic initContainer as the first one using PVB to identify the volumes and not annotations",
			pod: builder.ForPod("ns-1", "my-pod").
				Volumes(
					builder.ForVolume("vol-1").PersistentVolumeClaimSource("pvc-1").Result(),
					builder.ForVolume("vol-2").PersistentVolumeClaimSource("pvc-2").Result(),
				).
				ObjectMeta(
					builder.WithAnnotations("snapshot.velero.io/not-used", "")).
				InitContainers(builder.ForContainer("first-container", "").Result()).
				Result(),
			podVolumeBackups: []*velerov1api.PodVolumeBackup{
				builder.ForPodVolumeBackup(veleroNs, "pvb-1").
					PodName("my-pod").
					Volume("vol-1").
					ObjectMeta(builder.WithLabels(velerov1api.BackupNameLabel, backupName)).
					SnapshotID("foo").
					Result(),
				builder.ForPodVolumeBackup(veleroNs, "pvb-2").
					PodName("my-pod").
					Volume("vol-2").
					ObjectMeta(builder.WithLabels(velerov1api.BackupNameLabel, backupName)).
					SnapshotID("foo").
					Result(),
			},
			want: builder.ForPod("ns-1", "my-pod").
				Volumes(
					builder.ForVolume("vol-1").PersistentVolumeClaimSource("pvc-1").Result(),
					builder.ForVolume("vol-2").PersistentVolumeClaimSource("pvc-2").Result(),
				).
				ObjectMeta(
					builder.WithAnnotations("snapshot.velero.io/not-used", "")).
				InitContainers(
					newResticInitContainerBuilder(initContainerImage(defaultImageBase), "").
						Resources(&resourceReqs).
						VolumeMounts(builder.ForVolumeMount("vol-1", "/restores/vol-1").Result(), builder.ForVolumeMount("vol-2", "/restores/vol-2").Result()).Result(),
					builder.ForContainer("first-container", "").Result()).
				Result(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset()
			clientsetVelero := velerofake.NewSimpleClientset()

			for _, podVolumeBackup := range tc.podVolumeBackups {
				_, err := clientsetVelero.VeleroV1().PodVolumeBackups(veleroNs).Create(podVolumeBackup)
				require.NoError(t, err)
			}

			unstructuredMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(tc.pod)
			require.NoError(t, err)

			input := &velero.RestoreItemActionExecuteInput{
				Item: &unstructured.Unstructured{
					Object: unstructuredMap,
				},
				Restore: builder.ForRestore(veleroNs, restoreName).
					Backup(backupName).
					Phase(velerov1api.RestorePhaseInProgress).
					Result(),
			}

			a := NewResticRestoreAction(
				logrus.StandardLogger(),
				clientset.CoreV1().ConfigMaps(veleroNs),
				clientsetVelero.VeleroV1().PodVolumeBackups(veleroNs),
			)

			// method under test
			res, err := a.Execute(input)
			assert.NoError(t, err)

			updatedPod := new(corev1api.Pod)
			require.NoError(t, runtime.DefaultUnstructuredConverter.FromUnstructured(res.UpdatedItem.UnstructuredContent(), updatedPod))

			for _, container := range tc.want.Spec.InitContainers {
				sort.Slice(container.VolumeMounts, func(i, j int) bool {
					return container.VolumeMounts[i].Name < container.VolumeMounts[j].Name
				})
			}
			for _, container := range updatedPod.Spec.InitContainers {
				sort.Slice(container.VolumeMounts, func(i, j int) bool {
					return container.VolumeMounts[i].Name < container.VolumeMounts[j].Name
				})
			}

			assert.Equal(t, tc.want, updatedPod)
		})
	}

}
