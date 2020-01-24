package dataMover

import (
	"context"
	"github.com/sirupsen/logrus"
	"github.com/vmware-tanzu/astrolabe/pkg/astrolabe"
	"github.com/vmware-tanzu/astrolabe/pkg/ivd"
	"github.com/vmware-tanzu/astrolabe/pkg/s3repository"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/utils"
)

type DataMover struct {
	logrus.FieldLogger
	ivdPETM *ivd.IVDProtectedEntityTypeManager
	s3PETM  *s3repository.ProtectedEntityTypeManager
}

func NewDataMoverFromCluster(logger logrus.FieldLogger) (*DataMover, error) {
	params := make(map[string]interface{})
	err := utils.RetrieveVcConfigSecret(params, logger)

	if err != nil {
		logger.Errorf("Could not retrieve vsphere credential from k8s secret with error message: %v", err)
		return nil, err
	}
	logger.Infof("DataMover: vSphere VC credential is retrieved")

	err = utils.RetrieveVSLFromVeleroBSLs(params, logger)
	if err != nil {
		logger.Errorf("Could not retrieve velero default backup location with error message: %v", err)
		return nil, err
	}
	logger.Infof("DataMover: Velero Backup Storage Location is retrieved, region=%v, bucket=%v", params["region"], params["bucket"])

	s3PETM, err := utils.GetS3PETMFromParamsMap(params, logger)
	if err != nil {
		logger.Errorf("Failed to get s3PETM from params map")
		return nil, err
	}
	logger.Infof("DataMover: Get s3PETM from the params map")

	ivdPETM, err := utils.GetIVDPETMFromParamsMap(params, logger)
	if err != nil {
		logger.Errorf("Failed to get ivdPETM from params map")
		return nil, err
	}
	logger.Infof("DataMover: Get ivdPETM from the params map")

	dataMover := DataMover{
		FieldLogger: logger,
		ivdPETM: ivdPETM,
		s3PETM:  s3PETM,
	}

	logger.Infof("DataMover is initialized")
	return &dataMover, nil
}

func (this *DataMover) CopyToRepo(peID astrolabe.ProtectedEntityID) (astrolabe.ProtectedEntityID, error) {
	this.Infof("Copying the snapshot, %s, from local to remote repository", peID.String())
	ctx := context.Background()
	updatedPE, err := this.ivdPETM.GetProtectedEntity(ctx, peID)
	if err != nil {
		this.Errorf("Failed to GetProtectedEntity for, %s, with error message, %v", peID.String(), err)
		return astrolabe.ProtectedEntityID{}, err
	}

	this.Infof("Ready to call s3 PETM copy API")
	s3PE, err := this.s3PETM.Copy(ctx, updatedPE, astrolabe.AllocateNewObject)
	this.Infof("Return from the call of s3 PETM copy API")
	if err != nil {
		this.Errorf("Failed at copying to remote repository for, %s, with error message, %v",
			peID.String(), err)
		return astrolabe.ProtectedEntityID{}, err
	}

	this.Infof("Protected Entity, %s, was just copied from local to remote repository as, %s", peID.String(), s3PE.GetID().String())
	return s3PE.GetID(), nil
}

func (this *DataMover) CopyFromRepo(peID astrolabe.ProtectedEntityID) (astrolabe.ProtectedEntityID, error) {
	this.Infof("Copying the snapshot, %s, from remote repository to local", peID.String())
	ctx := context.Background()
	pe, err := this.s3PETM.GetProtectedEntity(ctx, peID)
	if err != nil {
		this.Errorf("Failed to GetProtectedEntity for, %s, with error message, %v", peID.String(), err)
		return astrolabe.ProtectedEntityID{}, err
	}

	this.Infof("Ready to call ivd PETM copy API")
	ivdPE, err := this.ivdPETM.Copy(ctx, pe, astrolabe.AllocateNewObject)
	this.Infof("Return from the call of ivd PETM copy API")
	if err != nil {
		this.Errorf("Failed to copy from remote repository for, %s, with error message, %v", peID.String(), err)
		return astrolabe.ProtectedEntityID{}, err
	}

	this.Infof("Protected Entity, %s, was just copied from remote repository to local as, %s", peID.String(), ivdPE.GetID().String())
	return ivdPE.GetID(), nil
}