package vsphere

import (
	"context"
	"github.com/sirupsen/logrus"
	"github.com/vmware/govmomi/cns"
	cnstypes "github.com/vmware/govmomi/cns/types"
	"github.com/vmware/govmomi/object"
)

// CnsManager provides functionality to manage volumes.
type CnsManager struct {
	logger        logrus.FieldLogger
	virtualCenter *VirtualCenter
	cnsClient     *cns.Client
}

// GetCnsManager returns the Manager instance.
func GetCnsManager(ctx context.Context, vc *VirtualCenter, cnsClient *cns.Client, logger logrus.FieldLogger) (*CnsManager, error) {
	logger.Infof("Initializing new CnsManager...")
	cnsManagerInstance := &CnsManager{
		virtualCenter: vc,
		cnsClient:     cnsClient,
		logger:        logger,
	}
	return cnsManagerInstance, nil
}

func (this *CnsManager) CreateVolume(ctx context.Context, createSpecList []cnstypes.CnsVolumeCreateSpec) (*object.Task, error) {
	invokeCount := 0
	var createVolumeTask *object.Task
	cachedCnsClient := this.cnsClient
	var err error
	for invokeCount <= DefaultAuthErrorRetryCount {
		invokeCount++
		createVolumeTask, err = cachedCnsClient.CreateVolume(ctx, createSpecList)
		if err != nil {
			cachedCnsClient, err = this.processError(invokeCount, err)
			if err != nil {
				return nil, err
			}
		} else {
			break
		}
	}
	return createVolumeTask, nil
}

func (this *CnsManager) DeleteVolume(ctx context.Context, volumeIDList []cnstypes.CnsVolumeId, deleteDisk bool) (*object.Task, error) {
	invokeCount := 0
	var deleteVolumeTask *object.Task
	cachedCnsClient := this.cnsClient
	var err error
	for invokeCount <= DefaultAuthErrorRetryCount {
		invokeCount++
		deleteVolumeTask, err = cachedCnsClient.DeleteVolume(ctx, volumeIDList, deleteDisk)
		if err != nil {
			cachedCnsClient, err = this.processError(invokeCount, err)
			if err != nil {
				return nil, err
			}
		} else {
			break
		}
	}
	return deleteVolumeTask, nil
}

func (this *CnsManager) QueryVolume(ctx context.Context, queryFilter cnstypes.CnsQueryFilter) (*cnstypes.CnsQueryResult, error) {
	invokeCount := 0
	var queryResult *cnstypes.CnsQueryResult
	cachedCnsClient := this.cnsClient
	var err error
	for invokeCount <= DefaultAuthErrorRetryCount {
		invokeCount++
		queryResult, err = cachedCnsClient.QueryVolume(ctx, queryFilter)
		if err != nil {
			cachedCnsClient, err = this.processError(invokeCount, err)
			if err != nil {
				return nil, err
			}
		} else {
			break
		}
	}
	return queryResult, nil
}

func (this *CnsManager) processError(invokeCount int, apiError error) (*cns.Client, error) {
	log := this.logger
	var refreshedCnsClient *cns.Client
	var err error
	if invokeCount > DefaultAuthErrorRetryCount {
		log.Errorf("Exceeded retry count for auth errors, err: %v", apiError)
		return nil, apiError
	}
	if CheckForVcAuthFaults(apiError, log) {
		// On auth fault re-initiate a connect and try again.
		_, refreshedCnsClient, err = this.virtualCenter.Connect(context.Background())
		if err != nil {
			log.Errorf("Failed to re-connect vcenter on auth errors..")
			return nil, err
		}
		log.Infof("Successfully re-initiated vcenter connection")
	} else {
		// Return the error on actual api error conditions.
		return nil, apiError
	}
	return refreshedCnsClient, nil
}

func (this *CnsManager) ResetManager(vcenter *VirtualCenter, cnsClient *cns.Client) {
	log := this.logger
	this.virtualCenter = vcenter
	this.cnsClient = cnsClient
	log.Infof("Done resetting CnsManager")
}
