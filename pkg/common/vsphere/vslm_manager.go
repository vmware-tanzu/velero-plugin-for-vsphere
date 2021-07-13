package vsphere

import (
	"context"
	"github.com/sirupsen/logrus"
	vim "github.com/vmware/govmomi/vim25/types"
	vslm_vsom "github.com/vmware/govmomi/vslm"
	vslmtypes "github.com/vmware/govmomi/vslm/types"
)

// VslmManager provides functionality to manage fcds.
type VslmManager struct {
	logger        logrus.FieldLogger
	virtualCenter *VirtualCenter
	vsom          *vslm_vsom.GlobalObjectManager
}

// GetVslmManager returns the Manager instance.
func GetVslmManager(
	ctx context.Context,
	vc *VirtualCenter,
	vsom *vslm_vsom.GlobalObjectManager,
	logger logrus.FieldLogger) (*VslmManager, error) {
	logger.Infof("Initializing new VslmManager...")
	vslmManagerInstance := &VslmManager{
		virtualCenter: vc,
		vsom:          vsom,
		logger:        logger,
	}
	return vslmManagerInstance, nil
}

func (this *VslmManager) CreateSnapshot(ctx context.Context, id vim.ID, description string) (*vslm_vsom.Task, error) {
	invokeCount := 0
	var createSnapshotTask *vslm_vsom.Task
	var err error
	cachedVsom := this.vsom
	for invokeCount <= DefaultAuthErrorRetryCount {
		invokeCount++
		createSnapshotTask, err = cachedVsom.CreateSnapshot(ctx, id, description)
		if err != nil {
			cachedVsom, err = this.processError(invokeCount, err)
			if err != nil {
				return nil, err
			}
		} else {
			break
		}
	}
	return createSnapshotTask, nil
}

func (this *VslmManager) DeleteSnapshot(ctx context.Context, id vim.ID, snapshotToDelete vim.ID) (*vslm_vsom.Task, error) {
	invokeCount := 0
	var deleteSnapshotTask *vslm_vsom.Task
	cachedVsom := this.vsom
	var err error
	for invokeCount <= DefaultAuthErrorRetryCount {
		invokeCount++
		deleteSnapshotTask, err = cachedVsom.DeleteSnapshot(ctx, id, snapshotToDelete)
		if err != nil {
			cachedVsom, err = this.processError(invokeCount, err)
			if err != nil {
				return nil, err
			}
		} else {
			break
		}
	}
	return deleteSnapshotTask, nil
}

func (this *VslmManager) RetrieveSnapshotInfo(ctx context.Context, id vim.ID) ([]vim.VStorageObjectSnapshotInfoVStorageObjectSnapshot, error) {
	invokeCount := 0
	var retreiveSnapInfoResults []vim.VStorageObjectSnapshotInfoVStorageObjectSnapshot
	cachedVsom := this.vsom
	var err error
	for invokeCount <= DefaultAuthErrorRetryCount {
		invokeCount++
		retreiveSnapInfoResults, err = cachedVsom.RetrieveSnapshotInfo(ctx, id)
		if err != nil {
			cachedVsom, err = this.processError(invokeCount, err)
			if err != nil {
				return nil, err
			}
		} else {
			break
		}
	}
	return retreiveSnapInfoResults, nil
}

func (this *VslmManager) Retrieve(ctx context.Context, id vim.ID) (*vim.VStorageObject, error) {
	invokeCount := 0
	var vStorageObject *vim.VStorageObject
	cachedVsom := this.vsom
	var err error
	for invokeCount <= DefaultAuthErrorRetryCount {
		invokeCount++
		vStorageObject, err = cachedVsom.Retrieve(ctx, id)
		if err != nil {
			cachedVsom, err = this.processError(invokeCount, err)
			if err != nil {
				return nil, err
			}
		} else {
			break
		}
	}
	return vStorageObject, nil
}

func (this *VslmManager) RetrieveSnapshotDetails(ctx context.Context, id vim.ID, snapshotId vim.ID) (*vim.VStorageObjectSnapshotDetails, error) {
	invokeCount := 0
	var snapshotDetails *vim.VStorageObjectSnapshotDetails
	cachedVsom := this.vsom
	var err error
	for invokeCount <= DefaultAuthErrorRetryCount {
		invokeCount++
		snapshotDetails, err = cachedVsom.RetrieveSnapshotDetails(ctx, id, snapshotId)
		if err != nil {
			cachedVsom, err = this.processError(invokeCount, err)
			if err != nil {
				return nil, err
			}
		} else {
			break
		}
	}
	return snapshotDetails, nil
}

func (this *VslmManager) UpdateMetadata(ctx context.Context, id vim.ID, metadata []vim.KeyValue, deleteKeys []string) (*vslm_vsom.Task, error) {
	invokeCount := 0
	var updateMetadatatTask *vslm_vsom.Task
	cachedVsom := this.vsom
	var err error
	for invokeCount <= DefaultAuthErrorRetryCount {
		invokeCount++
		updateMetadatatTask, err = cachedVsom.UpdateMetadata(ctx, id, metadata, deleteKeys)
		if err != nil {
			cachedVsom, err = this.processError(invokeCount, err)
			if err != nil {
				return nil, err
			}
		} else {
			break
		}
	}
	return updateMetadatatTask, nil
}

func (this *VslmManager) RetrieveMetadata(ctx context.Context, id vim.ID, snapshotID *vim.ID, prefix string) ([]vim.KeyValue, error) {
	invokeCount := 0
	var retrieveMetadata []vim.KeyValue
	cachedVsom := this.vsom
	var err error
	for invokeCount <= DefaultAuthErrorRetryCount {
		invokeCount++
		retrieveMetadata, err = cachedVsom.RetrieveMetadata(ctx, id, snapshotID, prefix)
		if err != nil {
			cachedVsom, err = this.processError(invokeCount, err)
			if err != nil {
				return nil, err
			}
		} else {
			break
		}
	}
	return retrieveMetadata, nil
}

func (this *VslmManager) CreateDiskFromSnapshot(ctx context.Context, id vim.ID, snapshotId vim.ID, name string, profile []vim.VirtualMachineProfileSpec, crypto *vim.CryptoSpec, path string) (*vslm_vsom.Task, error) {
	invokeCount := 0
	var createDiskFromSnapshotTask *vslm_vsom.Task
	cachedVsom := this.vsom
	var err error
	for invokeCount <= DefaultAuthErrorRetryCount {
		invokeCount++
		createDiskFromSnapshotTask, err = cachedVsom.CreateDiskFromSnapshot(ctx, id, snapshotId, name, profile, crypto, path)
		if err != nil {
			cachedVsom, err = this.processError(invokeCount, err)
			if err != nil {
				return nil, err
			}
		} else {
			break
		}
	}
	return createDiskFromSnapshotTask, nil
}

func (this *VslmManager) ListObjectsForSpec(ctx context.Context, query []vslmtypes.VslmVsoVStorageObjectQuerySpec, maxResult int32) (*vslmtypes.VslmVsoVStorageObjectQueryResult, error) {
	invokeCount := 0
	var queryResult *vslmtypes.VslmVsoVStorageObjectQueryResult
	cachedVsom := this.vsom
	var err error
	for invokeCount <= DefaultAuthErrorRetryCount {
		invokeCount++
		queryResult, err = cachedVsom.ListObjectsForSpec(ctx, query, maxResult)
		if err != nil {
			cachedVsom, err = this.processError(invokeCount, err)
			if err != nil {
				return nil, err
			}
		} else {
			break
		}
	}
	return queryResult, nil
}

func (this *VslmManager) CreateDisk(ctx context.Context, spec vim.VslmCreateSpec) (*vslm_vsom.Task, error) {
	invokeCount := 0
	var createDiskTask *vslm_vsom.Task
	cachedVsom := this.vsom
	var err error
	for invokeCount <= DefaultAuthErrorRetryCount {
		invokeCount++
		createDiskTask, err = cachedVsom.CreateDisk(ctx, spec)
		if err != nil {
			cachedVsom, err = this.processError(invokeCount, err)
			if err != nil {
				return nil, err
			}
		} else {
			break
		}
	}
	return createDiskTask, nil
}

func (this *VslmManager) Clone(ctx context.Context, id vim.ID, spec vim.VslmCloneSpec) (*vslm_vsom.Task, error) {
	invokeCount := 0
	var cloneTask *vslm_vsom.Task
	cachedVsom := this.vsom
	var err error
	for invokeCount <= DefaultAuthErrorRetryCount {
		invokeCount++
		cloneTask, err = cachedVsom.Clone(ctx, id, spec)
		if err != nil {
			cachedVsom, err = this.processError(invokeCount, err)
			if err != nil {
				return nil, err
			}
		} else {
			break
		}
	}
	return cloneTask, nil
}

func (this *VslmManager) Delete(ctx context.Context, id vim.ID) (*vslm_vsom.Task, error) {
	invokeCount := 0
	var deleteTask *vslm_vsom.Task
	cachedVsom := this.vsom
	var err error
	for invokeCount <= DefaultAuthErrorRetryCount {
		invokeCount++
		deleteTask, err = cachedVsom.Delete(ctx, id)
		if err != nil {
			cachedVsom, err = this.processError(invokeCount, err)
			if err != nil {
				return nil, err
			}
		} else {
			break
		}
	}
	return deleteTask, nil
}

func (this *VslmManager) processError(invokeCount int, apiError error) (*vslm_vsom.GlobalObjectManager, error) {
	log := this.logger
	var refreshedVsom *vslm_vsom.GlobalObjectManager
	var err error
	if invokeCount > DefaultAuthErrorRetryCount {
		log.Errorf("Exceeded retry count for auth errors, err: %v", apiError)
		return nil, apiError
	}
	if CheckForVcAuthFaults(apiError, log) {
		// On auth fault re-initiate a connect and try again.
		refreshedVsom, _, err = this.virtualCenter.Connect(context.Background())
		if err != nil {
			log.Errorf("Failed to re-connect vcenter on auth errors..")
			return nil, err
		}
		log.Infof("Successfully re-initiated vcenter connection")
	} else {
		// Return the error on actual api error conditions.
		return nil, apiError
	}
	return refreshedVsom, nil
}

// ResetManager helps set new manager instance and VC configuration
func (this *VslmManager) ResetManager(vcenter *VirtualCenter, vsom *vslm_vsom.GlobalObjectManager) {
	log := this.logger
	this.virtualCenter = vcenter
	this.vsom = vsom
	log.Infof("Done resetting VslmManager")
}
