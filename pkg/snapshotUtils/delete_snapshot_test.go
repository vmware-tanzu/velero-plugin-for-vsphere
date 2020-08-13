package snapshotUtils

import (
	"context"
	"github.com/sirupsen/logrus"
	backupdriverv1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/backupdriver/v1"
	backupdriverTypedV1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned/typed/backupdriver/v1"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"testing"
	"time"
)


/**
Pre-Requirements:
The test is intended to be triggered from a Guest Cluster.

Steps:

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

	backupdriverClient, err := backupdriverTypedV1.NewForConfig(config)
	if err != nil {
		t.Fatalf("Could not create clientset")
	}
	// Setup Logger
	logger := logrus.New()
	formatter := new(logrus.TextFormatter)
	formatter.TimestampFormat = time.RFC3339Nano
	formatter.FullTimestamp = true
	logger.SetFormatter(formatter)
	logger.SetLevel(logrus.DebugLevel)

	// Use pre-existing snapshot-id
	// snapshotID: pvc:test-ns/dynamic-block-pvc:snap-27e0f071-b11b-46a3-9657-d4c8087d7079
	// backpRepository: br-9a7e7499-a283-4f84-80b6-2bb9c8d725d1
	backupRepositoryName := "br-9a7e7499-a283-4f84-80b6-2bb9c8d725d1"
	snapshotID := "pvc:test-ns/dynamic-block-pvc:snap-27e0f071-b11b-46a3-9657-d4c8087d7079"
	backupRepository := NewBackupRepository(backupRepositoryName)
	namespace := "test-ns"

	deleteSnapshotRef, err := DeleteSnapshotRef(ctx, backupdriverClient, snapshotID, namespace, *backupRepository, []backupdriverv1.DeleteSnapshotPhase{backupdriverv1.DeleteSnapshotPhaseCompleted, backupdriverv1.DeleteSnapshotPhaseFailed}, logger)
	if err != nil {
		t.Fatalf("Received error during DeleteSnapshotRef %v", err)
	}
	if deleteSnapshotRef.Status.Phase != backupdriverv1.DeleteSnapshotPhaseCompleted {
		t.Fatalf("Failed to process delete snapshot %v", err)
	} else {
		logger.Infof("PASS!")
	}
}