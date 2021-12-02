package util

import (
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	"testing"
)

func Test_ItemToCRDName(t *testing.T) {
	accessor := meta.NewAccessor()

	backupUnstructuredMock := unstructured.Unstructured{}
	accessor.SetKind(&backupUnstructuredMock, "Backup")
	accessor.SetAPIVersion(&backupUnstructuredMock, "velero.io/v1")

	backupCRDName, err := UnstructuredToCRDName(&backupUnstructuredMock)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, backupCRDName, "backups.velero.io")

	repositoryUnstructuredMock := unstructured.Unstructured{}
	accessor.SetKind(&repositoryUnstructuredMock, "ResticRepository")
	accessor.SetAPIVersion(&repositoryUnstructuredMock, "velero.io/v1")

	repositoryCRDName, err := UnstructuredToCRDName(&repositoryUnstructuredMock)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, repositoryCRDName, "resticrepositories.velero.io")

	pvcUnstructuredMock := unstructured.Unstructured{}
	accessor.SetKind(&pvcUnstructuredMock, "PersistentVolumeClaim")
	accessor.SetAPIVersion(&pvcUnstructuredMock, "v1")

	pvcCRDName, err := UnstructuredToCRDName(&pvcUnstructuredMock)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, pvcCRDName, "persistentvolumeclaims")
}

var (
	csiStorageClass = "vsphere-csi-sc"
)

func TestGetPVForPVC(t *testing.T) {
	boundPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-csi-pvc",
			Namespace: "default",
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{},
			},
			StorageClassName: &csiStorageClass,
			VolumeName:       "test-csi-7d28e566-ade7-4ed6-9e15-2e44d2fbcc08",
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Phase:       corev1.ClaimBound,
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Capacity:    corev1.ResourceList{},
		},
	}
	matchingPV := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-csi-7d28e566-ade7-4ed6-9e15-2e44d2fbcc08",
		},
		Spec: corev1.PersistentVolumeSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Capacity:    corev1.ResourceList{},
			ClaimRef: &corev1.ObjectReference{
				Kind:            "PersistentVolumeClaim",
				Name:            "test-csi-pvc",
				Namespace:       "default",
				ResourceVersion: "1027",
				UID:             "7d28e566-ade7-4ed6-9e15-2e44d2fbcc08",
			},
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					Driver: "hostpath.csi.k8s.io",
					FSType: "ext4",
					VolumeAttributes: map[string]string{
						"storage.kubernetes.io/csiProvisionerIdentity": "1582049697841-8081-hostpath.csi.k8s.io",
					},
					VolumeHandle: "e61f2b48-527a-11ea-b54f-cab6317018f1",
				},
			},
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimDelete,
			StorageClassName:              csiStorageClass,
		},
		Status: corev1.PersistentVolumeStatus{
			Phase: corev1.VolumeBound,
		},
	}

	pvcWithNoVolumeName := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "no-vol-pvc",
			Namespace: "default",
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{},
			},
			StorageClassName: &csiStorageClass,
		},
		Status: corev1.PersistentVolumeClaimStatus{},
	}

	unboundPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "unbound-pvc",
			Namespace: "default",
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{},
			},
			StorageClassName: &csiStorageClass,
			VolumeName:       "test-csi-7d28e566-ade7-4ed6-9e15-2e44d2fbcc08",
		},
		Status: corev1.PersistentVolumeClaimStatus{
			Phase:       corev1.ClaimPending,
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Capacity:    corev1.ResourceList{},
		},
	}

	testCases := []struct {
		name        string
		inPVC       *corev1.PersistentVolumeClaim
		expectError bool
		expectedPV  *corev1.PersistentVolume
	}{
		{
			name:        "should find PV matching the PVC",
			inPVC:       boundPVC,
			expectError: false,
			expectedPV:  matchingPV,
		},
		{
			name:        "should fail to find PV for PVC with no volumeName",
			inPVC:       pvcWithNoVolumeName,
			expectError: true,
			expectedPV:  nil,
		},
		{
			name:        "should fail to find PV for PVC not in bound phase",
			inPVC:       unboundPVC,
			expectError: true,
			expectedPV:  nil,
		},
	}

	objs := []runtime.Object{boundPVC, matchingPV, pvcWithNoVolumeName, unboundPVC}
	fakeClient := fake.NewSimpleClientset(objs...)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualPV, actualError := GetPVForPVC(tc.inPVC, fakeClient.CoreV1())

			if tc.expectError {
				assert.NotNil(t, actualError, "Want error; Got nil error")
				assert.Nilf(t, actualPV, "Want PV: nil; Got PV: %q", actualPV)
				return
			}

			assert.Nilf(t, actualError, "Want: nil error; Got: %v", actualError)
			assert.Equalf(t, actualPV.Name, tc.expectedPV.Name, "Want PV with name %q; Got PV with name %q", tc.expectedPV.Name, actualPV.Name)
		})
	}
}

func TestGetPodsUsingPVC(t *testing.T) {
	objs := []runtime.Object{
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod1",
				Namespace: "default",
			},
			Spec: corev1.PodSpec{
				Volumes: []corev1.Volume{
					{
						Name: "csi-vol1",
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "csi-pvc1",
							},
						},
					},
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod2",
				Namespace: "default",
			},
			Spec: corev1.PodSpec{
				Volumes: []corev1.Volume{
					{
						Name: "csi-vol1",
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "csi-pvc1",
							},
						},
					},
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod3",
				Namespace: "default",
			},
			Spec: corev1.PodSpec{
				Volumes: []corev1.Volume{
					{
						Name: "csi-vol1",
						VolumeSource: corev1.VolumeSource{
							EmptyDir: &corev1.EmptyDirVolumeSource{},
						},
					},
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod1",
				Namespace: "awesome-ns",
			},
			Spec: corev1.PodSpec{
				Volumes: []corev1.Volume{
					{
						Name: "csi-vol1",
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "csi-pvc1",
							},
						},
					},
				},
			},
		},
	}
	fakeClient := fake.NewSimpleClientset(objs...)

	testCases := []struct {
		name             string
		pvcNamespace     string
		pvcName          string
		expectedPodCount int
	}{
		{
			name:             "should find exactly 2 pods using the PVC",
			pvcNamespace:     "default",
			pvcName:          "csi-pvc1",
			expectedPodCount: 2,
		},
		{
			name:             "should find exactly 1 pod using the PVC",
			pvcNamespace:     "awesome-ns",
			pvcName:          "csi-pvc1",
			expectedPodCount: 1,
		},
		{
			name:             "should find 0 pods using the PVC",
			pvcNamespace:     "default",
			pvcName:          "unused-pvc",
			expectedPodCount: 0,
		},
		{
			name:             "should find 0 pods in non-existent namespace",
			pvcNamespace:     "does-not-exist",
			pvcName:          "csi-pvc1",
			expectedPodCount: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualPods, err := GetPodsUsingPVC(tc.pvcNamespace, tc.pvcName, fakeClient.CoreV1())
			assert.Nilf(t, err, "Want error=nil; Got error=%v", err)
			assert.Equalf(t, len(actualPods), tc.expectedPodCount, "unexpected number of pods in result; Want: %d; Got: %d", tc.expectedPodCount, len(actualPods))
		})
	}
}

func TestGetPodVolumeNameForPVC(t *testing.T) {
	testCases := []struct {
		name               string
		pod                corev1.Pod
		pvcName            string
		expectError        bool
		expectedVolumeName string
	}{
		{
			name: "should get volume name for pod with multuple PVCs",
			pod: corev1.Pod{
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: "csi-vol1",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "csi-pvc1",
								},
							},
						},
						{
							Name: "csi-vol2",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "csi-pvc2",
								},
							},
						},
						{
							Name: "csi-vol3",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "csi-pvc3",
								},
							},
						},
					},
				},
			},
			pvcName:            "csi-pvc2",
			expectedVolumeName: "csi-vol2",
			expectError:        false,
		},
		{
			name: "should get volume name from pod using exactly one PVC",
			pod: corev1.Pod{
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: "csi-vol1",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "csi-pvc1",
								},
							},
						},
					},
				},
			},
			pvcName:            "csi-pvc1",
			expectedVolumeName: "csi-vol1",
			expectError:        false,
		},
		{
			name: "should return error for pod with no PVCs",
			pod: corev1.Pod{
				Spec: corev1.PodSpec{},
			},
			pvcName:     "csi-pvc2",
			expectError: true,
		},
		{
			name: "should return error for pod with no matching PVC",
			pod: corev1.Pod{
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: "csi-vol1",
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: "csi-pvc1",
								},
							},
						},
					},
				},
			},
			pvcName:     "mismatch-pvc",
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualVolumeName, err := GetPodVolumeNameForPVC(tc.pod, tc.pvcName)
			if tc.expectError && err == nil {
				assert.NotNil(t, err, "Want error; Got nil error")
				return
			}
			assert.Equalf(t, tc.expectedVolumeName, actualVolumeName, "unexpected podVolumename returned. Want %s; Got %s", tc.expectedVolumeName, actualVolumeName)
		})
	}
}

func TestContains(t *testing.T) {
	testCases := []struct {
		name           string
		inSlice        []string
		inKey          string
		expectedResult bool
	}{
		{
			name:           "should find the key",
			inSlice:        []string{"key1", "key2", "key3", "key4", "key5"},
			inKey:          "key3",
			expectedResult: true,
		},
		{
			name:           "should not find the key in non-empty slice",
			inSlice:        []string{"key1", "key2", "key3", "key4", "key5"},
			inKey:          "key300",
			expectedResult: false,
		},
		{
			name:           "should not find key in empty slice",
			inSlice:        []string{},
			inKey:          "key300",
			expectedResult: false,
		},
		{
			name:           "should not find key in nil slice",
			inSlice:        nil,
			inKey:          "key300",
			expectedResult: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualResult := Contains(tc.inSlice, tc.inKey)
			assert.Equal(t, tc.expectedResult, actualResult)
		})
	}
}

func TestIsPVCBackedUpByRestic(t *testing.T) {
	objs := []runtime.Object{
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod1",
				Namespace: "default",
			},
			Spec: corev1.PodSpec{
				Volumes: []corev1.Volume{
					{
						Name: "csi-vol1",
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "csi-pvc1",
							},
						},
					},
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod2",
				Namespace: "default",
				Annotations: map[string]string{
					"backup.velero.io/backup-volumes": "csi-vol1",
				},
			},
			Spec: corev1.PodSpec{
				Volumes: []corev1.Volume{
					{
						Name: "csi-vol1",
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "csi-pvc1",
							},
						},
					},
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod3",
				Namespace: "default",
			},
			Spec: corev1.PodSpec{
				Volumes: []corev1.Volume{
					{
						Name: "csi-vol1",
						VolumeSource: corev1.VolumeSource{
							EmptyDir: &corev1.EmptyDirVolumeSource{},
						},
					},
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "awesome-pod-1",
				Namespace: "awesome-ns",
			},
			Spec: corev1.PodSpec{
				Volumes: []corev1.Volume{
					{
						Name: "csi-vol1",
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "awesome-csi-pvc1",
							},
						},
					},
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "awesome-pod-2",
				Namespace: "awesome-ns",
			},
			Spec: corev1.PodSpec{
				Volumes: []corev1.Volume{
					{
						Name: "csi-vol1",
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "awesome-csi-pvc1",
							},
						},
					},
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod1",
				Namespace: "restic-ns",
				Annotations: map[string]string{
					"backup.velero.io/backup-volumes": "csi-vol1",
				},
			},
			Spec: corev1.PodSpec{
				Volumes: []corev1.Volume{
					{
						Name: "csi-vol1",
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "csi-pvc1",
							},
						},
					},
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pod2",
				Namespace: "restic-ns",
				Annotations: map[string]string{
					"backup.velero.io/backup-volumes": "csi-vol1",
				},
			},
			Spec: corev1.PodSpec{
				Volumes: []corev1.Volume{
					{
						Name: "csi-vol1",
						VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: "csi-pvc1",
							},
						},
					},
				},
			},
		},
	}
	fakeClient := fake.NewSimpleClientset(objs...)

	testCases := []struct {
		name                        string
		inPVCNamespace              string
		inPVCName                   string
		expectedIsResticUsed        bool
		defaultVolumeBackupToRestic bool
	}{
		{
			name:                        "2 pods using PVC, 1 pod using restic",
			inPVCNamespace:              "default",
			inPVCName:                   "csi-pvc1",
			expectedIsResticUsed:        true,
			defaultVolumeBackupToRestic: false,
		},
		{
			name:                        "2 pods using PVC, 2 pods using restic",
			inPVCNamespace:              "restic-ns",
			inPVCName:                   "csi-pvc1",
			expectedIsResticUsed:        true,
			defaultVolumeBackupToRestic: false,
		},
		{
			name:                        "2 pods using PVC, 0 pods using restic",
			inPVCNamespace:              "awesome-ns",
			inPVCName:                   "awesome-csi-pvc1",
			expectedIsResticUsed:        false,
			defaultVolumeBackupToRestic: false,
		},
		{
			name:                        "0 pods using PVC",
			inPVCNamespace:              "default",
			inPVCName:                   "does-not-exist",
			expectedIsResticUsed:        false,
			defaultVolumeBackupToRestic: false,
		},
		{
			name:                        "2 pods using PVC, using restic using restic by default",
			inPVCNamespace:              "awesome-ns",
			inPVCName:                   "awesome-csi-pvc1",
			expectedIsResticUsed:        true,
			defaultVolumeBackupToRestic: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualIsResticUsed, _ := IsPVCBackedUpByRestic(tc.inPVCNamespace, tc.inPVCName, fakeClient.CoreV1(), tc.defaultVolumeBackupToRestic)
			assert.Equal(t, tc.expectedIsResticUsed, actualIsResticUsed)
		})
	}
}

func TestIsMigratedCSIVolume(t *testing.T) {
	migratedPV := corev1.PersistentVolume{
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				VsphereVolume: &corev1.VsphereVirtualDiskVolumeSource{
					VolumePath: "fakePath",
				},
			},
		},
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"pv.kubernetes.io/migrated-to": "csi.vsphere.vmware.com",
			},
		},
	}

	vcpPV := corev1.PersistentVolume{
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				VsphereVolume: &corev1.VsphereVirtualDiskVolumeSource{
					VolumePath: "fakePath",
				},
			},
		},
	}

	csiPV := corev1.PersistentVolume{
		Spec: corev1.PersistentVolumeSpec{
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					Driver: "hostpath.csi.k8s.io",
					FSType: "ext4",
					VolumeAttributes: map[string]string{
						"storage.kubernetes.io/csiProvisionerIdentity": "1582049697841-8081-hostpath.csi.k8s.io",
					},
					VolumeHandle: "e61f2b48-527a-11ea-b54f-cab6317018f1",
				},
			},
		},
	}
	testCases := []struct {
		name                        string
		pv                          *corev1.PersistentVolume
		expectedValidMigratedVolume bool
	}{
		{
			name:                        "valid migrated csi volume",
			pv:                          &migratedPV,
			expectedValidMigratedVolume: true,
		},
		{
			name:                        "legacy vcp volume",
			pv:                          &vcpPV,
			expectedValidMigratedVolume: false,
		},
		{
			name:                        "standard csi volume",
			pv:                          &csiPV,
			expectedValidMigratedVolume: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualIsMigratedCSIVolume := IsMigratedCSIVolume(tc.pv)
			assert.Equal(t, tc.expectedValidMigratedVolume, actualIsMigratedCSIVolume)
		})
	}
}
