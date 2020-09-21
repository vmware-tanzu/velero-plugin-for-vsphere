package paravirt

import (
	"context"
	"encoding/base64"
	"fmt"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/constants"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/utils"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/vmware-tanzu/astrolabe/pkg/astrolabe"
	"github.com/vmware-tanzu/astrolabe/pkg/pvc"
	astrolabe_pvc "github.com/vmware-tanzu/astrolabe/pkg/pvc"
	"github.com/vmware-tanzu/astrolabe/pkg/util"
	backupdriverv1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/backupdriver/v1"
	backupdriverTypedV1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned/typed/backupdriver/v1"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/snapshotUtils"
	v1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	ParaVirtPETypePrefix = "paravirt"
	ParaVirtPETypeSep    = "-"
)

type ParaVirtEntityType string

const (
	ParaVirtEntityTypePersistentVolume  ParaVirtEntityType = "pv"
	ParaVirtEntityTypeVirtualMachine    ParaVirtEntityType = "vm"
	ParaVirtEntityTypePersistentService ParaVirtEntityType = "ps"
)

const (
	SnapshotParamBackupRepository = "BackupRepository"
	SnapshotParamSvcSnapshotName  = "SvcSnapshotName"
	SnapshotParamBackupName       = "BackupName"
)

type ParaVirtProtectedEntityTypeManager struct {
	entityType            ParaVirtEntityType
	gcKubeClientSet       *kubernetes.Clientset
	gcBackupDriverClient  *backupdriverTypedV1.BackupdriverV1Client
	svcKubeClientSet      *kubernetes.Clientset
	svcBackupDriverClient *backupdriverTypedV1.BackupdriverV1Client // we might want to change to BackupdriverV1Interface later
	svcNamespace          string
	s3Config              astrolabe.S3Config
	logger                logrus.FieldLogger
}

const (
	CSIDriverName = "csi.vsphere.vmware.com"
)

var _ astrolabe.ProtectedEntityTypeManager = (*ParaVirtProtectedEntityTypeManager)(nil)

func NewParaVirtProtectedEntityTypeManagerFromConfig(params map[string]interface{}, s3Config astrolabe.S3Config, logger logrus.FieldLogger) (*ParaVirtProtectedEntityTypeManager, error) {
	var err error
	var config *rest.Config
	// Get the PE Type
	entityType := params["entityType"].(ParaVirtEntityType)
	// Retrieve the rest config for guest cluster
	config, ok := params["restConfig"].(*rest.Config)
	if !ok {
		masterURL, _ := util.GetStringFromParamsMap(params, "masterURL", logger)
		kubeconfigPath, _ := util.GetStringFromParamsMap(params, "kubeconfigPath", logger)
		config, err = clientcmd.BuildConfigFromFlags(masterURL, kubeconfigPath)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}
	gcKubeClientSet, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// Retrieve the rest config for supervisor cluster
	svcConfig, ok := params["svcConfig"].(*rest.Config)
	if !ok {
		masterURL, _ := util.GetStringFromParamsMap(params, "svcMasterURL", logger)
		kubeconfigPath, _ := util.GetStringFromParamsMap(params, "svcKubeconfigPath", logger)
		config, err = clientcmd.BuildConfigFromFlags(masterURL, kubeconfigPath)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}
	svcKubeClientSet, err := kubernetes.NewForConfig(svcConfig)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	svcBackupDriverClient, err := backupdriverTypedV1.NewForConfig(svcConfig)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	gcBackupDriverClient, err := backupdriverTypedV1.NewForConfig(config)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	// Get the supervisor namespace where the paravirtualized PETM is running at
	svcNamespace := params["svcNamespace"].(string)

	// Fill in ParaVirt PETM
	return &ParaVirtProtectedEntityTypeManager{
		entityType:            entityType,
		gcKubeClientSet:       gcKubeClientSet,
		gcBackupDriverClient:  gcBackupDriverClient,
		svcKubeClientSet:      svcKubeClientSet,
		svcBackupDriverClient: svcBackupDriverClient,
		svcNamespace:          svcNamespace,
		s3Config:              s3Config,
		logger:                logger,
	}, nil
}

func (this *ParaVirtProtectedEntityTypeManager) GetTypeName() string {
	// e.g. "paravirt-pv"
	return ParaVirtPETypePrefix + ParaVirtPETypeSep + string(this.entityType)
}

func (this *ParaVirtProtectedEntityTypeManager) GetProtectedEntity(ctx context.Context, id astrolabe.ProtectedEntityID) (astrolabe.ProtectedEntity, error) {
	retPE, err := newParaVirtProtectedEntity(this, id)
	if err != nil {
		return nil, err
	}
	return retPE, nil
}

func (this *ParaVirtProtectedEntityTypeManager) GetProtectedEntities(ctx context.Context) ([]astrolabe.ProtectedEntityID, error) {
	if this.entityType != ParaVirtEntityTypePersistentVolume {
		return nil, errors.Errorf("The PE type, %v, is not supported", this.entityType)
	}

	pvList, err := this.gcKubeClientSet.CoreV1().PersistentVolumes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "Could not list PVs")
	}

	retPEIDs := make([]astrolabe.ProtectedEntityID, 0)
	for _, pv := range pvList.Items {
		if pv.Spec.CSI == nil || pv.Spec.CSI.Driver != pvc.VSphereCSIProvisioner {
			continue
		}
		retPEIDs = append(retPEIDs, astrolabe.NewProtectedEntityID(this.GetTypeName(), pv.Name))
	}

	return retPEIDs, nil
}

func (this *ParaVirtProtectedEntityTypeManager) Copy(ctx context.Context, pe astrolabe.ProtectedEntity, params map[string]map[string]interface{}, options astrolabe.CopyCreateOptions) (astrolabe.ProtectedEntity, error) {
	panic("implement me")
}

func (this *ParaVirtProtectedEntityTypeManager) CopyFromInfo(ctx context.Context, info astrolabe.ProtectedEntityInfo, params map[string]map[string]interface{}, options astrolabe.CopyCreateOptions) (astrolabe.ProtectedEntity, error) {
	panic("implement me")
}

func (this *ParaVirtProtectedEntityTypeManager) getDataTransports(id astrolabe.ProtectedEntityID) ([]astrolabe.DataTransport, []astrolabe.DataTransport, []astrolabe.DataTransport, error) {
	// TODO: placeholder that need to be revisited
	data := []astrolabe.DataTransport{}

	mdS3Transport, err := astrolabe.NewS3MDTransportForPEID(id, this.s3Config)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "Could not create S3 md transport")
	}

	md := []astrolabe.DataTransport{
		mdS3Transport,
	}

	combinedS3Transport, err := astrolabe.NewS3CombinedTransportForPEID(id, this.s3Config)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "Could not create S3 combined transport")
	}

	combined := []astrolabe.DataTransport{
		combinedS3Transport,
	}

	return data, md, combined, nil
}

// CreateFromMetadata creates CloneFromSnapshot CR in the Supervisor Cluster
func (this *ParaVirtProtectedEntityTypeManager) CreateFromMetadata(ctx context.Context, metadata []byte, sourceSnapshotID astrolabe.ProtectedEntityID, componentSourcePETM astrolabe.ProtectedEntityTypeManager, cloneFromSnapshotNamespace string, cloneFromSnapshotName string, backupRepositoryName string) (astrolabe.ProtectedEntity, error) {
	this.logger.Infof("CreateFromMetadata called on Paravirtualized PETM. sourceSnapshotID: %s. cloneFromSnapshot: %s/%s, backupRepositoryName: %s", sourceSnapshotID.String(), cloneFromSnapshotNamespace, cloneFromSnapshotName, backupRepositoryName)

	// Get backupRepository from Guest, set backRepositoryName to backupRepositoryObj.SvcBackupRepositoryName
	var backupRepo *snapshotUtils.BackupRepository
	if backupRepositoryName != "" && backupRepositoryName != constants.WithoutBackupRepository {
		backupRepositoryCR, err := utils.GetBackupRepositoryFromBackupRepositoryName(backupRepositoryName)
		if err != nil {
			this.logger.Errorf("Failed to get BackupRepository from BackupRepositoryName %s: %v", backupRepositoryName, err)
			return nil, errors.Wrapf(err, "failed to retrieve backupRepository")
		}
		if backupRepositoryCR.SvcBackupRepositoryName != "" && backupRepositoryCR.SvcBackupRepositoryName != constants.WithoutBackupRepository {
			this.logger.Info("BackupRepositoryName in Supervisor: %s", backupRepositoryCR.SvcBackupRepositoryName)
			backupRepositoryName = backupRepositoryCR.SvcBackupRepositoryName
		}
		backupRepo = snapshotUtils.NewBackupRepository(backupRepositoryName)
	}

	// sourceSnapshotID from CloneFromSnapshot in Guest is in this format:
	// pvc:test-ns-xtayual/test-pvc:cGFyYXZpcnQtcHY6cHZjLTE1ZDBhYzhmLTQxOGYtNGU1ZC05OWMxLWIxMjcyNjNlMjA1ODphWFprT2pGbE5EWmlZalJrTFdJelpqQXROREJrTlMwNVkyRTRMVE5pWVdVMlpqVTVOVGsxTlRwa01tVXlNamN4TmkwNVlUWXhMVFEyT0dFdFlqQmhZeTB4T1RKa01tSmtNV0kzWmpV
	snapIDDecoded, err := decodeSnapshotID(sourceSnapshotID.GetSnapshotID(), this.logger)
	if err != nil {
		this.logger.Errorf("Failed to retrieve decoded snapshot id for creating a CloneFromSnapshot CR in the supervisor cluster. sourceSnapshotID: %s", sourceSnapshotID.String())
		return nil, errors.Wrapf(err, "failed to retrieve decoded snapshot id")
	}
	this.logger.Info("CreateFromMetadata: decoded snapshotID: %s", snapIDDecoded)

	apiGroup := ""
	kind := "PersistentVolumeClaim"

	// Decode metadata, change namespace of PVC
	// from Guest Cluster namespace to Supervisor Cluster namespace,
	// and encode again before calling CreateFromMetadata
	// NOTE(xyang): We need to make assumption that StorageClass is already created
	// in Supervisor and Synchronized to Guest Cluster
	// StorageClass in Guest must have StorageClass name of Supervisor
	// parameters:
	//   svStorageClass: gc-storage-profile
	svcPVC := v1.PersistentVolumeClaim{}
	err = svcPVC.Unmarshal(metadata)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal metadata to get PVC")
	}
	// Construct Supervisor Cluster PVC name
	pvcUUID, err := uuid.NewRandom()
	if err != nil {
		// NOTE: svcPVC is marshaled from metadata which is originally
		// from the Guest Cluster PVC
		return nil, errors.Wrapf(err, "Could not generate PVC name in the Supervisor Cluster for Guest Cluster PVC %s/%s",
			svcPVC.Name, svcPVC.Namespace)
	}
	svcPVC.Name = svcPVC.Name[4:] + "-" + pvcUUID.String()
	svcPVC.Namespace = this.svcNamespace
	svcStorageClassName := ""
	if svcPVC.Spec.StorageClassName != nil {
		svcStorageClassName = *svcPVC.Spec.StorageClassName
	}
	this.logger.Infof("StorageClassName is %s in Supervisor PVC: %s/%s", svcStorageClassName, svcPVC.Name, svcPVC.Namespace)
	svcPVCData, err := svcPVC.Marshal()
	if err != nil {
		return nil, errors.Wrapf(err, "Could not marshal SVC PVC data for %s/%s",
			svcPVC.Name, svcPVC.Namespace)
	}

	// We need to construct the snapshotID in CloneFromSnapshot for Supervisor in this format:
	// pvc:test-gc-e2e-demo-ns/test-pvc:aXZkOjFlNDZiYjRkLWIzZjAtNDBkNS05Y2E4LTNiYWU2ZjU5NTk1NTpkMmUyMjcxNi05YTYxLTQ2OGEtYjBhYy0xOTJkMmJkMWI3ZjU
	// aXZkOjFlNDZiYjRkLWIzZjAtNDBkNS05Y2E4LTNiYWU2ZjU5NTk1NTpkMmUyMjcxNi05YTYxLTQ2OGEtYjBhYy0xOTJkMmJkMWI3ZjU will be decoded to ivd:1e46bb4d-b3f0-40d5-9ca8-3bae6f595955:ea4e347a-be29-4e5b-a626-725b83f168fcbase64 by PVC PETM
	newSnapshotID := "pvc:" + svcPVC.Namespace + "/" + svcPVC.Name + ":" + snapIDDecoded
	this.logger.Infof("CreateFromMetadata: constructed new SnapshotID: %s", newSnapshotID)

	// Build a CloneFromSnapshot API object for Supervisor Cluster
	// and wait for its phase to become Completed, Failed, or Canceled.
	// metadata should be []byte form of PVC so supervisor cluster
	// PVC PETM will call CreateFromMetadata which will create a PVC
	// in the Supervisor Cluster
	this.logger.Info("Creating a CloneFromSnapshot CR")
	svcClone, err := snapshotUtils.CloneFromSnapshopRef(ctx, this.svcBackupDriverClient, newSnapshotID, svcPVCData, &apiGroup, kind, this.svcNamespace, *backupRepo, []backupdriverv1.ClonePhase{backupdriverv1.ClonePhaseCompleted, backupdriverv1.ClonePhaseFailed, backupdriverv1.ClonePhaseCanceled}, this.logger)
	this.logger.Infof("CreateFromMetadata: finished waiting for CloneFromSnapshot's status to be completed, failed, or canceled in the Supervisor Cluster")

	if err != nil {
		this.logger.Errorf("Failed to create a cloneFromSnapshot CR: %v", err)
		return nil, err
	}

	if svcClone.Status.Phase == backupdriverv1.ClonePhaseFailed {
		this.logger.Errorf("CloneFromSnapshot CR %s/%s failed in the Supervisor Cluster", this.svcNamespace, svcClone.Name)
		return nil, fmt.Errorf("CloneFromSnapshot failed in the Supervisor Cluster: %s/%s", svcClone.Namespace, svcClone.Name)
	} else if svcClone.Status.Phase == backupdriverv1.ClonePhaseCanceled {
		this.logger.Errorf("CloneFromSnapshot CR %s/%s is canceled in the Supervisor Cluster", this.svcNamespace, svcClone.Name)
		return nil, fmt.Errorf("CloneFromSnapshot is canceled: %s/%s in the Supervisor Cluster", svcClone.Namespace, svcClone.Name)
	}
	this.logger.Infof("CreateFromMetadata: CloneFromSnapshot %s/%s is completed in Supervisor Cluster. Phase: %v", svcClone.Namespace, svcClone.Name, svcClone.Status.Phase)

	// Get a fresh Supervisor PVC object
	svcPvcUpdated, err := this.svcKubeClientSet.CoreV1().PersistentVolumeClaims(this.svcNamespace).Get(context.TODO(), svcPVC.Name, metav1.GetOptions{})
	if err != nil {
		this.logger.Errorf("Failed to get PVC %s/%s from Supervisor Cluster: %v", this.svcNamespace, svcPVC.Name, err)
		return nil, errors.Wrapf(err, "Failed to get PVC from Supervisor Cluster")
	}
	svcPVC = *svcPvcUpdated

	// Create a PV in Guest Cluster that points to PVC Name in Supervisor Cluster (static provisioning)
	namespace := cloneFromSnapshotNamespace
	pvcName := svcPVC.Name
	pvUUID, err := uuid.NewRandom()
	if err != nil {
		return nil, errors.Wrapf(err, "Could not generate PV name in the Guest Cluster for Supervisor Cluster PVC %s/%s",
			svcPVC.Name, svcPVC.Namespace)
	}
	pv := &v1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: pvcName + pvUUID.String(),
		},
		Spec: v1.PersistentVolumeSpec{
			AccessModes: svcPVC.Spec.AccessModes,
			Capacity:    svcPVC.Status.Capacity,
			PersistentVolumeSource: v1.PersistentVolumeSource{
				CSI: &v1.CSIPersistentVolumeSource{
					Driver:       CSIDriverName,
					VolumeHandle: svcPVC.Name,
				},
			},
			ClaimRef: &v1.ObjectReference{
				Namespace: namespace,
				Name:      pvcName,
			},
		},
	}

	if pv, err = this.gcKubeClientSet.CoreV1().PersistentVolumes().Create(context.TODO(), pv, metav1.CreateOptions{}); err == nil || apierrs.IsAlreadyExists(err) {
		// Save succeeded.
		if err != nil {
			this.logger.Infof("PV %s already exists, reusing", pv.Name)
			err = nil
		} else {
			this.logger.Infof("PV %s saved", pv.Name)
		}
	}
	this.logger.Infof("CreateFromMetadata: PV %s created in the guest cluster", pv.Name)

	// Construct PVC, setting PVC's VolumeName to PV Name
	accessModes := svcPVC.Spec.AccessModes
	resources := svcPVC.Spec.Resources
	volumeMode := svcPVC.Spec.VolumeMode
	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      pvcName,
		},
		Spec: v1.PersistentVolumeClaimSpec{
			AccessModes: accessModes,
			Resources:   resources,
			VolumeMode:  volumeMode,
			VolumeName:  pv.Name,
		},
	}

	// Create a PVC in Guest Cluster to statically bind to PV in Guest Cluster
	if pvc, err = this.gcKubeClientSet.CoreV1().PersistentVolumeClaims(namespace).Create(context.TODO(), pvc, metav1.CreateOptions{}); err == nil || apierrs.IsAlreadyExists(err) {
		// Save succeeded.
		if err != nil {
			this.logger.Infof("PVC %s/%s already exists, reusing", pvc.Namespace, pvc.Name)
			err = nil
		} else {
			this.logger.Infof("PVC %s/%s saved", pvc.Namespace, pvc.Name)
		}
	}
	this.logger.Infof("CreateFromMetadata: PVC %s/%s is created in the guest cluster", pvc.Namespace, pvc.Name)

	// Wait for PVC and PV to bind to each other
	err = astrolabe_pvc.WaitForPersistentVolumeClaimPhase(v1.ClaimBound, this.gcKubeClientSet, pvc.Namespace, pvc.Name, astrolabe_pvc.Poll, astrolabe_pvc.ClaimBindingTimeout, this.logger)
	if err != nil {
		return nil, fmt.Errorf("pvc %s/%s did not become Bound: %v", pvc.Namespace, pvc.Name, err)
	}

	this.logger.Infof("CreateFromMetadata: PVC %s/%s is bound to PV %s.", pvc.Namespace, pvc.Name, pv.Name)

	// Get CloneFromSnapshot from Guest
	gcClone, err := this.gcBackupDriverClient.CloneFromSnapshots(cloneFromSnapshotNamespace).Get(context.TODO(), cloneFromSnapshotName, metav1.GetOptions{})
	if err != nil {
		this.logger.Errorf("Failed to get the cloneFromSnapshot CR %s/%s in Guest Cluster: %v", cloneFromSnapshotNamespace, cloneFromSnapshotName, err)
		return nil, errors.Wrapf(err, "failed to get cloneFromSnapshot record in Guest Cluster")
	}

	// Update CloneFromSnapshot status in Guest based on CloneFromSnapshot from Supervisor
	clone := gcClone.DeepCopy()
	clone.Status.Phase = svcClone.Status.Phase
	clone.Status.Message = svcClone.Status.Message
	clone.Status.ResourceHandle = svcClone.Status.ResourceHandle.DeepCopy()
	_, err = this.gcBackupDriverClient.CloneFromSnapshots(cloneFromSnapshotNamespace).UpdateStatus(context.TODO(), clone, metav1.UpdateOptions{})
	if err != nil {
		this.logger.Errorf("CreateFromMetadata: Failed to update status of CloneFromSnapshot %s/%s to %v", cloneFromSnapshotNamespace, cloneFromSnapshotName, clone.Status.Phase)
		return nil, err
	}
	this.logger.Infof("CreateFromMetadata: CloneFromSnapshot %s/%s updated successfully to Phase %v", cloneFromSnapshotNamespace, cloneFromSnapshotName, clone.Status.Phase)

	peID := astrolabe_pvc.NewProtectedEntityIDFromPVCName(pvc.Namespace, pvc.Name)
	this.logger.Infof("CreateFromMetadata: generated peID: %s.", peID.String())

	pe, err := this.GetProtectedEntity(ctx, peID)
	if err != nil {
		this.logger.WithError(err).Errorf("Failed to get the ProtectedEntity from peID %s", peID.String())
		return nil, err
	}

	this.logger.Infof("CreateFromMetadata: retrieved ProtectedEntity for ID %s.", peID.String())
	return pe, nil
}

func decodeSnapshotID(snapshotID astrolabe.ProtectedEntitySnapshotID, logger logrus.FieldLogger) (string, error) {
	// Decode incoming snapshot ID until we retrieve the ivd snapshot-id.
	snapshotID64Str := snapshotID.String()
	snapshotIDBytes, err := base64.RawStdEncoding.DecodeString(snapshotID64Str)
	if err != nil {
		errorMsg := fmt.Sprintf("Could not decode snapshot ID encoded string %s", snapshotID64Str)
		logger.WithError(err).Error(errorMsg)
		return "", errors.Wrap(err, errorMsg)
	}
	decodedPEID, err := astrolabe.NewProtectedEntityIDFromString(string(snapshotIDBytes))
	if err != nil {
		errorMsg := fmt.Sprintf("Could not translate decoded snapshotID %s into pe-id", string(snapshotIDBytes))
		logger.WithError(err).Error(errorMsg)
		return "", errors.Wrap(err, errorMsg)
	}
	logger.Infof("Successfully translated snapshotID %s into pe-id: %s", string(snapshotIDBytes), decodedPEID.String())
	if decodedPEID.HasSnapshot() && decodedPEID.GetPeType() != "ivd" {
		logger.Infof("The translated pe-id is not ivd type, recurring for further decode")
		return decodeSnapshotID(decodedPEID.GetSnapshotID(), logger)
	}
	// Encode PEID before returning
	// Example: convert from ivd:1e46bb4d-b3f0-40d5-9ca8-3bae6f595955:ea4e347a-be29-4e5b-a626-725b83f168fcbase64 to aXZkOjFlNDZiYjRkLWIzZjAtNDBkNS05Y2E4LTNiYWU2ZjU5NTk1NTplYTRlMzQ3YS1iZTI5LTRlNWItYTYyNi03MjViODNmMTY4ZmM
	encodedSnapshotStr := base64.RawStdEncoding.EncodeToString([]byte(decodedPEID.String()))
	logger.Infof("Successfully encoded snapshot ID: %s", encodedSnapshotStr)
	return encodedSnapshotStr, nil
}
