/*
Copyright 2017 the Velero contributors.

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

package kube

import (
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kubeinformers "k8s.io/client-go/informers"

	"github.com/vmware-tanzu/velero/pkg/builder"
	"github.com/vmware-tanzu/velero/pkg/test"
	velerotest "github.com/vmware-tanzu/velero/pkg/test"
)

func TestNamespaceAndName(t *testing.T) {
	//TODO
}

func TestEnsureNamespaceExistsAndIsReady(t *testing.T) {
	tests := []struct {
		name           string
		expectNSFound  bool
		nsPhase        corev1.NamespacePhase
		nsDeleting     bool
		expectCreate   bool
		alreadyExists  bool
		expectedResult bool
	}{
		{
			name:           "namespace found, not deleting",
			expectNSFound:  true,
			expectedResult: true,
		},
		{
			name:           "namespace found, terminating phase",
			expectNSFound:  true,
			nsPhase:        corev1.NamespaceTerminating,
			expectedResult: false,
		},
		{
			name:           "namespace found, deletiontimestamp set",
			expectNSFound:  true,
			nsDeleting:     true,
			expectedResult: false,
		},
		{
			name:           "namespace not found, successfully created",
			expectCreate:   true,
			expectedResult: true,
		},
		{
			name:           "namespace not found initially, create returns already exists error, returned namespace is ready",
			alreadyExists:  true,
			expectedResult: true,
		},
		{
			name:           "namespace not found initially, create returns already exists error, returned namespace is terminating",
			alreadyExists:  true,
			nsPhase:        corev1.NamespaceTerminating,
			expectedResult: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			namespace := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
			}

			if test.nsPhase != "" {
				namespace.Status.Phase = test.nsPhase
			}

			if test.nsDeleting {
				namespace.SetDeletionTimestamp(&metav1.Time{Time: time.Now()})
			}

			timeout := time.Millisecond

			nsClient := &velerotest.FakeNamespaceClient{}
			defer nsClient.AssertExpectations(t)

			if test.expectNSFound {
				nsClient.On("Get", "test", metav1.GetOptions{}).Return(namespace, nil)
			} else {
				nsClient.On("Get", "test", metav1.GetOptions{}).Return(&corev1.Namespace{}, k8serrors.NewNotFound(schema.GroupResource{Resource: "namespaces"}, "test"))
			}

			if test.alreadyExists {
				nsClient.On("Create", namespace).Return(namespace, k8serrors.NewAlreadyExists(schema.GroupResource{Resource: "namespaces"}, "test"))
			}

			if test.expectCreate {
				nsClient.On("Create", namespace).Return(namespace, nil)
			}

			result, _ := EnsureNamespaceExistsAndIsReady(namespace, nsClient, timeout)

			assert.Equal(t, test.expectedResult, result)
		})
	}

}

type harness struct {
	*test.APIServer

	log logrus.FieldLogger
}

func newHarness(t *testing.T) *harness {
	t.Helper()

	return &harness{
		APIServer: test.NewAPIServer(t),
		log:       logrus.StandardLogger(),
	}
}

// TestGetVolumeDirectorySuccess tests that the GetVolumeDirectory function
// returns a volume's name or a volume's name plus '/mount' when a PVC is present.
func TestGetVolumeDirectorySuccess(t *testing.T) {
	tests := []struct {
		name string
		pod  *corev1.Pod
		pvc  *corev1.PersistentVolumeClaim
		pv   *corev1.PersistentVolume
		want string
	}{
		{
			name: "Non-CSI volume with a PVC/PV returns the volume's name",
			pod:  builder.ForPod("ns-1", "my-pod").Volumes(builder.ForVolume("my-vol").PersistentVolumeClaimSource("my-pvc").Result()).Result(),
			pvc:  builder.ForPersistentVolumeClaim("ns-1", "my-pvc").VolumeName("a-pv").Result(),
			pv:   builder.ForPersistentVolume("a-pv").Result(),
			want: "a-pv",
		},
		{
			name: "CSI volume with a PVC/PV appends '/mount' to the volume name",
			pod:  builder.ForPod("ns-1", "my-pod").Volumes(builder.ForVolume("my-vol").PersistentVolumeClaimSource("my-pvc").Result()).Result(),
			pvc:  builder.ForPersistentVolumeClaim("ns-1", "my-pvc").VolumeName("a-pv").Result(),
			pv:   builder.ForPersistentVolume("a-pv").CSI("csi.test.com", "provider-volume-id").Result(),
			want: "a-pv/mount",
		},
		{
			name: "CSI volume mounted without a PVC appends '/mount' to the volume name",
			pod:  builder.ForPod("ns-1", "my-pod").Volumes(builder.ForVolume("my-vol").CSISource("csi.test.com").Result()).Result(),
			want: "my-vol/mount",
		},
		{
			name: "Non-CSI volume without a PVC returns the volume name",
			pod:  builder.ForPod("ns-1", "my-pod").Volumes(builder.ForVolume("my-vol").Result()).Result(),
			want: "my-vol",
		},
	}

	for _, tc := range tests {
		h := newHarness(t)

		pvcInformer := kubeinformers.NewSharedInformerFactoryWithOptions(h.KubeClient, 0, kubeinformers.WithNamespace("ns-1")).Core().V1().PersistentVolumeClaims()
		pvInformer := kubeinformers.NewSharedInformerFactory(h.KubeClient, 0).Core().V1().PersistentVolumes()

		if tc.pvc != nil {
			require.NoError(t, pvcInformer.Informer().GetStore().Add(tc.pvc))
		}
		if tc.pv != nil {
			require.NoError(t, pvInformer.Informer().GetStore().Add(tc.pv))
		}

		// Function under test
		dir, err := GetVolumeDirectory(tc.pod, tc.pod.Spec.Volumes[0].Name, pvcInformer.Lister(), pvInformer.Lister())

		require.NoError(t, err)
		assert.Equal(t, tc.want, dir)
	}
}
