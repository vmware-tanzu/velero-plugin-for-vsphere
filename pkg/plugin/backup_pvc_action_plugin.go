package plugin

import (
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	backupdriverv1api "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/backupdriver/v1"
	backupdriverClientSet "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned"
	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"github.com/vmware-tanzu/velero/pkg/plugin/velero"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
	"os"
	"time"
)

// PVCBackupItemAction is a backup item action plugin for Velero.
type NewPVCBackupItemAction struct {
	Log logrus.FieldLogger
}

// AppliesTo returns information indicating that the PVCBackupItemAction should be invoked to backup PVCs.
func (p *NewPVCBackupItemAction) AppliesTo() (velero.ResourceSelector, error) {
	p.Log.Info("PVCBackupItemAction AppliesTo for vSphere")

	return velero.ResourceSelector{
		IncludedResources: []string{"persistentvolumeclaims"},
	}, nil
}

const (
	PollTimeoutForSnapshot = 5 * time.Minute
	PollLogInterval = 30 * time.Second
)

// Execute recognizes PVCs backed by volumes provisioned by vSphere CNS block volumes
func (p *NewPVCBackupItemAction) Execute(item runtime.Unstructured, backup *velerov1api.Backup) (runtime.Unstructured, []velero.ResourceIdentifier, error) {
	p.Log.Info("Starting PVCBackupItemAction for vSphere")

	// Do nothing if volume snapshots have not been requested in this backup
	//if utils.IsSetToFalse(backup.Spec.SnapshotVolumes) {
	if backup.Spec.SnapshotVolumes != nil && *backup.Spec.SnapshotVolumes == false {
		p.Log.Infof("Volume snapshotting not requested for backup %s/%s", backup.Namespace, backup.Name)
		return item, nil, nil
	}

	var pvc corev1.PersistentVolumeClaim
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.UnstructuredContent(), &pvc); err != nil {
		return nil, nil, errors.WithStack(err)
	}

	// get the velero namespace and the rest config in k8s cluster
	veleroNs, exist := os.LookupEnv("VELERO_NAMESPACE")
	if !exist {
		errMsg := "Failed to lookup the ENV variable for velero namespace"
		p.Log.Error(errMsg)
		return nil, nil, errors.New(errMsg)
	}

	restConfig, err := rest.InClusterConfig()
	if err != nil {
		p.Log.Error("Failed to get the rest config in k8s cluster: %v", err)
		return nil, nil, errors.WithStack(err)
	}

	// look up within velero namespace for a backup repository claim that has access to the namespace of PVC
	selectedBRC, err := PickOneBackupRepositoryClaim(restConfig, veleroNs, pvc.Namespace)
	if err != nil {
		// No qualified backup repository exists. So, create a new one in the velero namespace.
		selectedBRC, err = CreateBackupRepositoryClaim(restConfig, veleroNs, pvc.Namespace)
	}

	if selectedBRC == nil {
		// TODO: fail it when the utils for backup repository are ready
		p.Log.Info("BackupRepositoryClaim related logic has not been implemented")
	}

	// create a snapshot CR from the input backup object
	snapshot := &backupdriverv1api.Snapshot{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "backupdriver-snapshot-" + pvc.Name + "-",
			Namespace:    pvc.Namespace,
		},
		Spec: backupdriverv1api.SnapshotSpec{
			TypedLocalObjectReference: corev1.TypedLocalObjectReference{
				APIGroup: &corev1.SchemeGroupVersion.Group,
				Kind: pvc.Kind,
				Name: pvc.Name,
			},
			BackupRepository: "", // TODO: replace it with the name of selected backup repository
		},
	}

	backupdriverClient, err := backupdriverClientSet.NewForConfig(restConfig)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	// create a snapshot request
	updatedSnapshot, err := backupdriverClient.BackupdriverV1().Snapshots(pvc.Namespace).Create(snapshot)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "error creating backupdriver snapshot")
	}
	// update status of snapshot request
	updatedSnapshot.Status.Phase = backupdriverv1api.SnapshotPhaseNew
	updatedSnapshot, err = backupdriverClient.BackupdriverV1().Snapshots(pvc.Namespace).UpdateStatus(updatedSnapshot)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "error updating the status of backupdriver snapshot")
	}
	p.Log.Infof("Created volumesnapshot %s/%s with status.phase %s", updatedSnapshot.Namespace, updatedSnapshot.Name, updatedSnapshot.Status.Phase)

	// wait until the snapshot phase turns to be "Snapshotted"
	lastPollLogTime := time.Now()
	err = wait.PollImmediate(time.Second, PollTimeoutForSnapshot, func() (bool, error) {
		infoLog := false
		if time.Now().Sub(lastPollLogTime) > PollLogInterval {
			infoLog = true
			p.Log.Infof("Polling snapshot request %s", updatedSnapshot.Name)
			lastPollLogTime = time.Now()
		}

		retrievedSnapshot, err := backupdriverClient.BackupdriverV1().Snapshots(pvc.Namespace).Get(updatedSnapshot.Name, metav1.GetOptions{})
		if err != nil {
			p.Log.Errorf("Retrieve snapshot request %s failed with err %v", updatedSnapshot.Name, err)
			return false, errors.WithStack(err)
		}

		if retrievedSnapshot.Status.Phase == backupdriverv1api.SnapshotPhaseSnapshotFailed {
			return false, errors.Errorf("Snapshot failed at the backup driver: %s", retrievedSnapshot.Status.Message)
		} else if retrievedSnapshot.Status.SnapshotID != "" {
			// plugin is supposed to wait until the PVC is snapshotted. However,
			// the status.phase of retrieved snapshot might not be just "snapshotted"
			// since the phase might be moved forward very quickly. So, it would be
			// better to verify whether the field, status.snapshotID, is empty since
			// it will be set once it is "snapshotted".
			p.Log.Infof("Snapshot request %s snapshotted with SnapshotID %s",
				retrievedSnapshot.Name, retrievedSnapshot.Status.SnapshotID)
			return true, nil
		} else {
			if infoLog {
				p.Log.Infof("Retrieve phase(%s) for snapshot request %s",
					retrievedSnapshot.Status.Phase, retrievedSnapshot.Name)
			}
			return false, nil
		}
	})
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	var additionalItems []velero.ResourceIdentifier

	pvcMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&pvc)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}

	return &unstructured.Unstructured{Object: pvcMap}, additionalItems, nil
}

// TODO: the following functions will be remove utils are ready
func CreateBackupRepositoryClaim(config *rest.Config, veleroNS string, pvcNS string) (*backupdriverv1api.BackupRepositoryClaim, error) {
	return nil, nil
}

func PickOneBackupRepositoryClaim(config *rest.Config, veleroNS string, pvcNS string) (*backupdriverv1api.BackupRepositoryClaim, error) {
	return nil, nil
}