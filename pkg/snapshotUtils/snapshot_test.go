package snapshotUtils

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	backupdriverv1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/backupdriver/v1alpha1"
	v1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned/typed/backupdriver/v1alpha1"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/utils"
	core_v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	"testing"
	"time"
)

func TestWaitForPhases(t *testing.T) {
	clientSet, err := createClientSet()

	if err != nil {
		_, ok := err.(ClientConfigNotFoundError)
		if ok {
			t.Skip(err)
		}
		t.Fatal(err)
	}
	apiGroup := "xyzzy"
	testSnapshot := backupdriverv1.Snapshot{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Snapshot",
			APIVersion: "cnsdp.vmware.com/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "dave-test-4",
		},
		Spec: backupdriverv1.SnapshotSpec{
			TypedLocalObjectReference: core_v1.TypedLocalObjectReference{
				APIGroup: &apiGroup,
				Kind:     "volume",
				Name:     "dave-volume",
			},
			BackupRepository: "test-repo",
			SnapshotCancel:   false,
		},
		Status: backupdriverv1.SnapshotStatus{
			Phase:   backupdriverv1.SnapshotPhaseNew,
			Message: "Snapshot done",
			Progress: backupdriverv1.SnapshotProgress{
				TotalBytes: 0,
				BytesDone:  0,
			},
			SnapshotID: "id",
			Metadata:   []byte("my metadata"),
		},
	}

	// set up logger
	logger := logrus.New()
	formatter := new(logrus.TextFormatter)
	formatter.TimestampFormat = time.RFC3339
	formatter.FullTimestamp = true
	logger.SetFormatter(formatter)

	err = clientSet.Snapshots("backup-driver").Delete(context.TODO(), testSnapshot.Name, metav1.DeleteOptions{})
	if err != nil {
		t.Fatalf("Delete error = %v\n", err)
	}
	writtenSnapshot, err := clientSet.Snapshots("backup-driver").Create(context.TODO(), &testSnapshot, metav1.CreateOptions{})

	testSnapshot.ObjectMeta = writtenSnapshot.ObjectMeta
	writtenSnapshot, err = clientSet.Snapshots("backup-driver").UpdateStatus(context.TODO(), &testSnapshot, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("writtenSnapshot =%v, err = %v", writtenSnapshot, err)
	}

	timeoutContext, cancelFunc := context.WithDeadline(context.Background(), time.Now().Add(time.Second*10))
	defer cancelFunc()
	updatedSnapshot, err := WaitForPhases(timeoutContext, clientSet, testSnapshot, []backupdriverv1.SnapshotPhase{backupdriverv1.SnapshotPhaseSnapshotted}, "backup-driver", logger)
	if err != nil {
		t.Fatalf("WaitForPhases returned err = %v\n", err)
	} else {
		fmt.Printf("WaitForPhases returned phase = %s\n", updatedSnapshot.Status.Phase)
	}
}

func TestWaitForClonePhases(t *testing.T) {
	clientSet, err := createClientSet()

	if err != nil {
		_, ok := err.(ClientConfigNotFoundError)
		if ok {
			t.Skip(err)
		}
		t.Fatal(err)
	}
	apiGroup := ""
	testClone := backupdriverv1.CloneFromSnapshot{
		TypeMeta: metav1.TypeMeta{
			Kind:       "CloneFromSnapshot",
			APIVersion: "cnsdp.vmware.com/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "dp-test-1",
		},
		Spec: backupdriverv1.CloneFromSnapshotSpec{
			SnapshotID:       "snapshot-1",
			Metadata:         []byte("my metadata"),
			APIGroup:         &apiGroup,
			Kind:             "PersistentVolumeClaim",
			BackupRepository: "test-repo",
			CloneCancel:      false,
		},
		Status: backupdriverv1.CloneStatus{
			Phase:   backupdriverv1.ClonePhaseNew,
			Message: "Clone from snapshot done",
			ResourceHandle: &core_v1.TypedLocalObjectReference{
				APIGroup: &apiGroup,
				Kind:     "volume",
				Name:     "dp-volume",
			},
		},
	}

	utils.AddVeleroExcludeLabelToObjectMeta(&testClone.ObjectMeta)

	// set up logger
	logger := logrus.New()
	formatter := new(logrus.TextFormatter)
	formatter.TimestampFormat = time.RFC3339
	formatter.FullTimestamp = true
	logger.SetFormatter(formatter)

	err = clientSet.Snapshots("backup-driver").Delete(context.TODO(), testClone.Name, metav1.DeleteOptions{})
	if err != nil {
		t.Fatalf("Delete error = %v\n", err)
	}
	writtenClone, err := clientSet.CloneFromSnapshots("backup-driver").Create(context.TODO(), &testClone, metav1.CreateOptions{})

	testClone.ObjectMeta = writtenClone.ObjectMeta
	writtenClone, err = clientSet.CloneFromSnapshots("backup-driver").UpdateStatus(context.TODO(), &testClone, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("writtenClone =%v, err = %v", writtenClone, err)
	}

	timeoutContext, cancelFunc := context.WithDeadline(context.Background(), time.Now().Add(time.Second*10))
	defer cancelFunc()
	updatedClone, err := WaitForClonePhases(timeoutContext, clientSet, testClone, []backupdriverv1.ClonePhase{backupdriverv1.ClonePhaseCompleted}, "backup-driver", logger)
	if err != nil {
		t.Fatalf("WaitForClonePhases returned err = %v\n", err)
	} else {
		fmt.Printf("WaitForClonePhases returned phase = %s\n", updatedClone.Status.Phase)
	}
}

/*
TODO - figure out how to advance the phase and status
*/
/*
func TestSnapshotRef(t *testing.T) {
	clientSet, err := createClientSet()

	if err != nil {
		t.Fatal(err)
	}

	apiGroup := "xyzzy"

	dummyVolumeName, err := uuid.NewRandom()
	if err != nil {
		t.Fatal(err)
	}
	objectToSnapshot := core_v1.TypedLocalObjectReference{
		APIGroup: &apiGroup,
		Kind:     "volume",
		Name:     dummyVolumeName.String(),
	}

	backupRepository := BackupRepository{
		backupRepository: "test-repo",
	}

	snapshot, err := SnapshopRef(context.Background(), clientSet, objectToSnapshot, "backup-driver", backupRepository,
		[]backupdriverv1.SnapshotPhase{backupdriverv1.SnapshotPhaseSnapshotted})
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("Snapshot created with name %s\n", snapshot.Name)
}
*/
func createClientSet() (*v1.BackupdriverV1alpha1Client, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	// if you want to change the loading rules (which files in which order), you can do so here

	configOverrides := &clientcmd.ConfigOverrides{}
	// if you want to change override values or bind them to flags, there are methods to help you

	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	config, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, NewClientConfigNotFoundError("Could not create client config")
	}

	clientset, err := v1.NewForConfig(config)

	if err != nil {
		return nil, errors.Wrap(err, "Could not create clientset")
	}
	return clientset, err
}

type ClientConfigNotFoundError struct {
	errMsg string
}

func (this ClientConfigNotFoundError) Error() string {
	return this.errMsg
}

func NewClientConfigNotFoundError(errMsg string) ClientConfigNotFoundError {
	err := ClientConfigNotFoundError{
		errMsg: errMsg,
	}
	return err
}
