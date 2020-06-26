package backupdriver

import (
	"context"
	"fmt"
	log "github.com/sirupsen/logrus"
	backupdriverv1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/backupdriver/v1"
	v1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned/typed/backupdriver/v1"
	core_v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	"testing"
	"time"
)

func TestWaitForPhases(t *testing.T) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	// if you want to change the loading rules (which files in which order), you can do so here

	configOverrides := &clientcmd.ConfigOverrides{}
	// if you want to change override values or bind them to flags, there are methods to help you

	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	config, err := kubeConfig.ClientConfig()
	if err != nil {
		log.Panic(err.Error())
	}

	clientset, err := v1.NewForConfig(config)

	/*
	clientset, err := kubernetes.NewForConfig(config)
	*/
	if err != nil {
		log.Panic(err.Error())
	}

	apiGroup := "xyzzy"
	testSnapshot := backupdriverv1.Snapshot{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Snapshot",
			APIVersion: "backupdriver.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:                       "dave-test-4",
		},
		Spec:       backupdriverv1.SnapshotSpec{
			TypedLocalObjectReference: core_v1.TypedLocalObjectReference{
				APIGroup: &apiGroup,
				Kind:     "volume",
				Name:     "dave-volume",
			},
			BackupRepository:          "test-repo",
			SnapshotCancel:            false,
		},
		Status:     backupdriverv1.SnapshotStatus{
			Phase:      backupdriverv1.SnapshotPhaseNew,
			Message:    "Snapshot done",
			Progress:   backupdriverv1.SnapshotProgress{
				TotalBytes: 0,
				BytesDone:  0,
			},
			SnapshotID: "id",
			Metadata:   []byte("my metadata"),
		},
	}

	err = clientset.Snapshots("backup-driver").Delete(testSnapshot.Name, nil)
	if err != nil {
		fmt.Printf("Delete error = %v\n", err)
	}
	writtenSnapshot, err := clientset.Snapshots("backup-driver").Create(&testSnapshot)

	testSnapshot.ObjectMeta = writtenSnapshot.ObjectMeta
	writtenSnapshot, err = clientset.Snapshots("backup-driver").UpdateStatus(&testSnapshot)
	if err != nil {
		fmt.Printf("writtenSnapshot =%v, err = %v", writtenSnapshot, err)
	}

	timeoutContext, _ := context.WithDeadline(context.Background(), time.Now().Add(time.Second * 10))
	endPhase, err := WaitForPhases(timeoutContext, clientset, testSnapshot, []backupdriverv1.SnapshotPhase{backupdriverv1.SnapshotPhaseSnapshotted})
	if err != nil {
		fmt.Printf("WaitForPhases returned err = %v\n", err)
	} else {
		fmt.Printf("WaitForPhases returned phase = %s\n", endPhase)
	}
}