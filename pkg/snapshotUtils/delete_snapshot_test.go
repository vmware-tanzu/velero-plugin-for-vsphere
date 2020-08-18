package snapshotUtils

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	backupdriverv1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/backupdriver/v1"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/backupdriver"
	backupdriverTypedV1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned/typed/backupdriver/v1"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/utils"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"testing"
	"time"
)

/**
End-to-End test for verifying DeleteSnapshot

Pre-Requirements:
1. The test is intended to be triggered from a Guest Cluster.
2. gc-storage-profile pre-exists in the inventory.
3. "default" BSL exists in velero.

*/
func TestDeleteSnapshot(t *testing.T) {
	ctx := context.Background()
	path := os.Getenv("KUBECONFIG")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("The KubeConfig file, %v, is not exist", path)
	}

	config, err := clientcmd.BuildConfigFromFlags("", path)
	if err != nil {
		t.Fatalf("Failed to build k8s config from kubeconfig file: %+v ", err)
	}

	clientSet, err := clientset.NewForConfig(config)
	if err != nil {
		t.Fatalf("Failed to create a ClientSet: %v. Exiting.", err)
	}

	backupdriverClient, err := backupdriverTypedV1.NewForConfig(config)
	if err != nil {
		t.Fatalf("Could not create clientset")
	}

	veleroNs, exist := os.LookupEnv("VELERO_NAMESPACE")
	if !exist {
		errMsg := "Failed to lookup the ENV variable for velero namespace"
		t.Fatalf(errMsg)
	}

	// Setup Logger
	logger := logrus.New()
	formatter := new(logrus.TextFormatter)
	formatter.TimestampFormat = time.RFC3339Nano
	formatter.FullTimestamp = true
	logger.SetFormatter(formatter)
	logger.SetLevel(logrus.DebugLevel)

	// Step 1: Create a namespace
	ns, err := uuid.NewRandom()
	if err != nil {
		t.Fatalf("Failed to generate random name for a namespace")
	}
	nsName := "ns-" + ns.String()
	nsSpec := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
	_, err = clientSet.CoreV1().Namespaces().Create(nsSpec)
	if err != nil {
		t.Fatalf("Failed to create namespace: %s", nsName)
	}

	// Step 2: Create a PVC
	storageClass := "gc-storage-profile"
	pvcName := "test-pvc"
	volumeMode := v1.PersistentVolumeBlock
	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: nsName,
		},
		Spec: v1.PersistentVolumeClaimSpec{
			AccessModes: []v1.PersistentVolumeAccessMode{v1.ReadWriteOnce},
			Resources: v1.ResourceRequirements{
				Requests: v1.ResourceList{
					v1.ResourceStorage: resource.MustParse("10M")},
			},
			StorageClassName: &storageClass,
			VolumeMode:       &volumeMode,
		},
	}
	pvcCreated, err := clientSet.CoreV1().PersistentVolumeClaims(nsName).Create(pvc)
	if err != nil {
		t.Fatalf("Failed to create PVC in namespace: %s", nsName)
	}
	// Wait till the pvc is bound.
	pvcBoundStatus := false
	for !pvcBoundStatus {
		pvcUpdated, err := clientSet.CoreV1().PersistentVolumeClaims(nsName).Get(pvcName, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("Failed to retrieve the PVC")
		}
		if pvcUpdated.Status.Phase == v1.ClaimBound {
			logger.Infof("PVC %s successfully bound", pvcName)
			pvcBoundStatus = true
		} else {
			logger.Infof("PVC %s not bound yet, sleeping 2s..", pvcName)
			time.Sleep(2 * time.Second)
		}
	}

	// Step 3: Calim a BackupRepository
	bslName := "default"
	repositoryParameters := make(map[string]string)
	err = utils.RetrieveParamsFromBSL(repositoryParameters, bslName, config, logger)
	if err != nil {
		t.Fatalf("Failed to translate BSL to repository parameters: %v", err)
	}
	backupRepositoryName, err := backupdriver.ClaimBackupRepository(ctx, utils.S3RepositoryDriver, repositoryParameters,
		[]string{pvc.Namespace}, veleroNs, backupdriverClient, logger)
	if err != nil {
		t.Fatalf("Failed to claim backup repository: %v", err)
	}
	backupRepository := NewBackupRepository(backupRepositoryName)

	objectToSnapshot := v1.TypedLocalObjectReference{
		APIGroup: &v1.SchemeGroupVersion.Group,
		Kind:     pvcCreated.Kind,
		Name:     pvcCreated.Name,
	}

	// Step 4: Create a snapshot
	logger.Info("Creating a Snapshot CR")
	updatedSnapshot, err := SnapshotRef(ctx, backupdriverClient, objectToSnapshot, pvc.Namespace, *backupRepository,
		[]backupdriverv1.SnapshotPhase{backupdriverv1.SnapshotPhaseSnapshotted, backupdriverv1.SnapshotPhaseSnapshotFailed}, logger)
	if err != nil {
		t.Fatalf("Failed to create a Snapshot CR: %v", err)
	}
	if updatedSnapshot.Status.Phase == backupdriverv1.SnapshotPhaseSnapshotFailed {
		errMsg := fmt.Sprintf("Failed to create a Snapshot CR: Phase=SnapshotFailed, err=%v", updatedSnapshot.Status.Message)
		t.Fatalf(errMsg)
	}

	// Sleeping for 10s to wait for upload to complete.
	logger.Infof("Sleeping 10s to wait for upload to be completed.")
	time.Sleep(10 * time.Second)

	// Step 5: Delete the Snapshot
	snapshotID := updatedSnapshot.Status.SnapshotID
	// Example Formats:
	// backpRepository: br-9a7e7499-a283-4f84-80b6-2bb9c8d725d1
	// snapshotID := "pvc:test-ns/dynamic-block-pvc:27e0f071-b11b-46a3-9657-d4c8087d7079"

	deleteSnapshotRef, err := DeleteSnapshotRef(ctx, backupdriverClient, snapshotID, nsName, *backupRepository,
		[]backupdriverv1.DeleteSnapshotPhase{backupdriverv1.DeleteSnapshotPhaseCompleted, backupdriverv1.DeleteSnapshotPhaseFailed}, logger)
	if err != nil {
		t.Fatalf("Received error during DeleteSnapshotRef %v", err)
	}
	if deleteSnapshotRef.Status.Phase != backupdriverv1.DeleteSnapshotPhaseCompleted {
		t.Fatalf("Failed to process delete snapshot %v", err)
	} else {
		logger.Infof("PASS!")
	}
}
