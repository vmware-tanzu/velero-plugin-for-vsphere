/*
Copyright 2018 the Velero contributors.

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

package restic

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1api "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"

	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"github.com/vmware-tanzu/velero/pkg/builder"
	"github.com/vmware-tanzu/velero/pkg/generated/clientset/versioned/fake"
	informers "github.com/vmware-tanzu/velero/pkg/generated/informers/externalversions"
	velerotest "github.com/vmware-tanzu/velero/pkg/test"
)

func TestGetVolumeBackupsForPod(t *testing.T) {
	tests := []struct {
		name             string
		podVolumeBackups []*velerov1api.PodVolumeBackup
		podAnnotations   map[string]string
		podName          string
		expected         map[string]string
	}{
		{
			name:           "nil annotations",
			podAnnotations: nil,
			expected:       nil,
		},
		{
			name:           "empty annotations",
			podAnnotations: make(map[string]string),
			expected:       nil,
		},
		{
			name:           "non-empty map, no snapshot annotation",
			podAnnotations: map[string]string{"foo": "bar"},
			expected:       nil,
		},
		{
			name:           "has snapshot annotation only, no suffix",
			podAnnotations: map[string]string{podAnnotationPrefix: "bar"},
			expected:       map[string]string{"": "bar"},
		},
		{
			name:           "has snapshot annotation only, with suffix",
			podAnnotations: map[string]string{podAnnotationPrefix + "foo": "bar"},
			expected:       map[string]string{"foo": "bar"},
		},
		{
			name:           "has snapshot annotation, with suffix",
			podAnnotations: map[string]string{"x": "y", podAnnotationPrefix + "foo": "bar", podAnnotationPrefix + "abc": "123"},
			expected:       map[string]string{"foo": "bar", "abc": "123"},
		},
		{
			name: "has snapshot annotation, with suffix, and also PVBs",
			podVolumeBackups: []*velerov1api.PodVolumeBackup{
				builder.ForPodVolumeBackup("velero", "pvb-1").PodName("TestPod").SnapshotID("bar").Volume("pvbtest1-foo").Result(),
				builder.ForPodVolumeBackup("velero", "pvb-2").PodName("TestPod").SnapshotID("123").Volume("pvbtest2-abc").Result(),
			},
			podName:        "TestPod",
			podAnnotations: map[string]string{"x": "y", podAnnotationPrefix + "foo": "bar", podAnnotationPrefix + "abc": "123"},
			expected:       map[string]string{"pvbtest1-foo": "bar", "pvbtest2-abc": "123"},
		},
		{
			name: "no snapshot annotation, but with PVBs",
			podVolumeBackups: []*velerov1api.PodVolumeBackup{
				builder.ForPodVolumeBackup("velero", "pvb-1").PodName("TestPod").SnapshotID("bar").Volume("pvbtest1-foo").Result(),
				builder.ForPodVolumeBackup("velero", "pvb-2").PodName("TestPod").SnapshotID("123").Volume("pvbtest2-abc").Result(),
			},
			podName:  "TestPod",
			expected: map[string]string{"pvbtest1-foo": "bar", "pvbtest2-abc": "123"},
		},
		{
			name: "no snapshot annotation, but with PVBs, some of which have snapshot IDs and some of which don't",
			podVolumeBackups: []*velerov1api.PodVolumeBackup{
				builder.ForPodVolumeBackup("velero", "pvb-1").PodName("TestPod").SnapshotID("bar").Volume("pvbtest1-foo").Result(),
				builder.ForPodVolumeBackup("velero", "pvb-2").PodName("TestPod").SnapshotID("123").Volume("pvbtest2-abc").Result(),
				builder.ForPodVolumeBackup("velero", "pvb-3").PodName("TestPod").Volume("pvbtest3-foo").Result(),
				builder.ForPodVolumeBackup("velero", "pvb-4").PodName("TestPod").Volume("pvbtest4-abc").Result(),
			},
			podName:  "TestPod",
			expected: map[string]string{"pvbtest1-foo": "bar", "pvbtest2-abc": "123"},
		},
		{
			name: "has snapshot annotation, with suffix, and with PVBs from current pod and a PVB from another pod",
			podVolumeBackups: []*velerov1api.PodVolumeBackup{
				builder.ForPodVolumeBackup("velero", "pvb-1").PodName("TestPod").SnapshotID("bar").Volume("pvbtest1-foo").Result(),
				builder.ForPodVolumeBackup("velero", "pvb-2").PodName("TestPod").SnapshotID("123").Volume("pvbtest2-abc").Result(),
				builder.ForPodVolumeBackup("velero", "pvb-3").PodName("TestAnotherPod").SnapshotID("xyz").Volume("pvbtest3-xyz").Result(),
			},
			podAnnotations: map[string]string{"x": "y", podAnnotationPrefix + "foo": "bar", podAnnotationPrefix + "abc": "123"},
			podName:        "TestPod",
			expected:       map[string]string{"pvbtest1-foo": "bar", "pvbtest2-abc": "123"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pod := &corev1api.Pod{}
			pod.Annotations = test.podAnnotations
			pod.Name = test.podName

			res := GetVolumeBackupsForPod(test.podVolumeBackups, pod)
			assert.Equal(t, test.expected, res)
		})
	}
}

func TestGetVolumesToBackup(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		expected    []string
	}{
		{
			name:        "nil annotations",
			annotations: nil,
			expected:    nil,
		},
		{
			name:        "no volumes to backup",
			annotations: map[string]string{"foo": "bar"},
			expected:    nil,
		},
		{
			name:        "one volume to backup",
			annotations: map[string]string{"foo": "bar", volumesToBackupAnnotation: "volume-1"},
			expected:    []string{"volume-1"},
		},
		{
			name:        "multiple volumes to backup",
			annotations: map[string]string{"foo": "bar", volumesToBackupAnnotation: "volume-1,volume-2,volume-3"},
			expected:    []string{"volume-1", "volume-2", "volume-3"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pod := &corev1api.Pod{}
			pod.Annotations = test.annotations

			res := GetVolumesToBackup(pod)

			// sort to ensure good compare of slices
			sort.Strings(test.expected)
			sort.Strings(res)

			assert.Equal(t, test.expected, res)
		})
	}
}

func TestGetSnapshotsInBackup(t *testing.T) {
	tests := []struct {
		name                  string
		podVolumeBackups      []velerov1api.PodVolumeBackup
		expected              []SnapshotIdentifier
		longBackupNameEnabled bool
	}{
		{
			name:             "no pod volume backups",
			podVolumeBackups: nil,
			expected:         nil,
		},
		{
			name: "no pod volume backups with matching label",
			podVolumeBackups: []velerov1api.PodVolumeBackup{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "foo", Labels: map[string]string{velerov1api.BackupNameLabel: "non-matching-backup-1"}},
					Spec: velerov1api.PodVolumeBackupSpec{
						Pod: corev1api.ObjectReference{Name: "pod-1", Namespace: "ns-1"},
					},
					Status: velerov1api.PodVolumeBackupStatus{SnapshotID: "snap-1"},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "bar", Labels: map[string]string{velerov1api.BackupNameLabel: "non-matching-backup-2"}},
					Spec: velerov1api.PodVolumeBackupSpec{
						Pod: corev1api.ObjectReference{Name: "pod-2", Namespace: "ns-2"},
					},
					Status: velerov1api.PodVolumeBackupStatus{SnapshotID: "snap-2"},
				},
			},
			expected: nil,
		},
		{
			name: "some pod volume backups with matching label",
			podVolumeBackups: []velerov1api.PodVolumeBackup{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "foo", Labels: map[string]string{velerov1api.BackupNameLabel: "non-matching-backup-1"}},
					Spec: velerov1api.PodVolumeBackupSpec{
						Pod: corev1api.ObjectReference{Name: "pod-1", Namespace: "ns-1"},
					},
					Status: velerov1api.PodVolumeBackupStatus{SnapshotID: "snap-1"},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "bar", Labels: map[string]string{velerov1api.BackupNameLabel: "non-matching-backup-2"}},
					Spec: velerov1api.PodVolumeBackupSpec{
						Pod: corev1api.ObjectReference{Name: "pod-2", Namespace: "ns-2"},
					},
					Status: velerov1api.PodVolumeBackupStatus{SnapshotID: "snap-2"},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "completed-pvb", Labels: map[string]string{velerov1api.BackupNameLabel: "backup-1"}},
					Spec: velerov1api.PodVolumeBackupSpec{
						Pod: corev1api.ObjectReference{Name: "pod-1", Namespace: "ns-1"},
					},
					Status: velerov1api.PodVolumeBackupStatus{SnapshotID: "snap-3"},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "completed-pvb-2", Labels: map[string]string{velerov1api.BackupNameLabel: "backup-1"}},
					Spec: velerov1api.PodVolumeBackupSpec{
						Pod: corev1api.ObjectReference{Name: "pod-1", Namespace: "ns-1"},
					},
					Status: velerov1api.PodVolumeBackupStatus{SnapshotID: "snap-4"},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "incomplete-or-failed-pvb", Labels: map[string]string{velerov1api.BackupNameLabel: "backup-1"}},
					Spec: velerov1api.PodVolumeBackupSpec{
						Pod: corev1api.ObjectReference{Name: "pod-1", Namespace: "ns-2"},
					},
					Status: velerov1api.PodVolumeBackupStatus{SnapshotID: ""},
				},
			},
			expected: []SnapshotIdentifier{
				{
					VolumeNamespace: "ns-1",
					SnapshotID:      "snap-3",
				},
				{
					VolumeNamespace: "ns-1",
					SnapshotID:      "snap-4",
				},
			},
		},
		{
			name:                  "some pod volume backups with matching label and backup name greater than 63 chars",
			longBackupNameEnabled: true,
			podVolumeBackups: []velerov1api.PodVolumeBackup{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "foo", Labels: map[string]string{velerov1api.BackupNameLabel: "non-matching-backup-1"}},
					Spec: velerov1api.PodVolumeBackupSpec{
						Pod: corev1api.ObjectReference{Name: "pod-1", Namespace: "ns-1"},
					},
					Status: velerov1api.PodVolumeBackupStatus{SnapshotID: "snap-1"},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "bar", Labels: map[string]string{velerov1api.BackupNameLabel: "non-matching-backup-2"}},
					Spec: velerov1api.PodVolumeBackupSpec{
						Pod: corev1api.ObjectReference{Name: "pod-2", Namespace: "ns-2"},
					},
					Status: velerov1api.PodVolumeBackupStatus{SnapshotID: "snap-2"},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "completed-pvb", Labels: map[string]string{velerov1api.BackupNameLabel: "the-really-long-backup-name-that-is-much-more-than-63-cha6ca4bc"}},
					Spec: velerov1api.PodVolumeBackupSpec{
						Pod: corev1api.ObjectReference{Name: "pod-1", Namespace: "ns-1"},
					},
					Status: velerov1api.PodVolumeBackupStatus{SnapshotID: "snap-3"},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "completed-pvb-2", Labels: map[string]string{velerov1api.BackupNameLabel: "backup-1"}},
					Spec: velerov1api.PodVolumeBackupSpec{
						Pod: corev1api.ObjectReference{Name: "pod-1", Namespace: "ns-1"},
					},
					Status: velerov1api.PodVolumeBackupStatus{SnapshotID: "snap-4"},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "incomplete-or-failed-pvb", Labels: map[string]string{velerov1api.BackupNameLabel: "backup-1"}},
					Spec: velerov1api.PodVolumeBackupSpec{
						Pod: corev1api.ObjectReference{Name: "pod-1", Namespace: "ns-2"},
					},
					Status: velerov1api.PodVolumeBackupStatus{SnapshotID: ""},
				},
			},
			expected: []SnapshotIdentifier{
				{
					VolumeNamespace: "ns-1",
					SnapshotID:      "snap-3",
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var (
				client          = fake.NewSimpleClientset()
				sharedInformers = informers.NewSharedInformerFactory(client, 0)
				pvbInformer     = sharedInformers.Velero().V1().PodVolumeBackups()
				veleroBackup    = &velerov1api.Backup{}
			)

			veleroBackup.Name = "backup-1"

			if test.longBackupNameEnabled {
				veleroBackup.Name = "the-really-long-backup-name-that-is-much-more-than-63-characters"
			}

			for _, pvb := range test.podVolumeBackups {
				require.NoError(t, pvbInformer.Informer().GetStore().Add(pvb.DeepCopy()))
			}

			res, err := GetSnapshotsInBackup(veleroBackup, pvbInformer.Lister())
			assert.NoError(t, err)

			// sort to ensure good compare of slices
			less := func(snapshots []SnapshotIdentifier) func(i, j int) bool {
				return func(i, j int) bool {
					if snapshots[i].VolumeNamespace == snapshots[j].VolumeNamespace {
						return snapshots[i].SnapshotID < snapshots[j].SnapshotID
					}
					return snapshots[i].VolumeNamespace < snapshots[j].VolumeNamespace
				}

			}

			sort.Slice(test.expected, less(test.expected))
			sort.Slice(res, less(res))

			assert.Equal(t, test.expected, res)
		})
	}
}

func TestTempCredentialsFile(t *testing.T) {
	var (
		secretInformer = cache.NewSharedIndexInformer(nil, new(corev1api.Secret), 0, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
		secretLister   = corev1listers.NewSecretLister(secretInformer.GetIndexer())
		fs             = velerotest.NewFakeFileSystem()
		secret         = &corev1api.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "velero",
				Name:      CredentialsSecretName,
			},
			Data: map[string][]byte{
				CredentialsKey: []byte("passw0rd"),
			},
		}
	)

	// secret not in lister: expect an error
	fileName, err := TempCredentialsFile(secretLister, "velero", "default", fs)
	assert.Error(t, err)

	// now add secret to lister
	require.NoError(t, secretInformer.GetStore().Add(secret))

	// secret in lister: expect temp file to be created with password
	fileName, err = TempCredentialsFile(secretLister, "velero", "default", fs)
	require.NoError(t, err)

	contents, err := fs.ReadFile(fileName)
	require.NoError(t, err)

	assert.Equal(t, "passw0rd", string(contents))
}
