/*
Copyright 2020 the Velero contributors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package dataMover

import (
	"context"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/backuprepository"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/constants"
	"sync"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/vmware-tanzu/astrolabe/pkg/astrolabe"
	"github.com/vmware-tanzu/astrolabe/pkg/ivd"
	"github.com/vmware-tanzu/astrolabe/pkg/s3repository"
	backupdriverv1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/backupdriver/v1"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/builder"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/utils"
)

type DataMover struct {
	logrus.FieldLogger
	ivdPETM             *ivd.IVDProtectedEntityTypeManager
	s3PETM              *s3repository.ProtectedEntityTypeManager
	inProgressCancelMap *sync.Map
}

func NewDataMoverFromCluster(params map[string]interface{}, externalDataMgr bool, logger logrus.FieldLogger) (*DataMover, error) {
	// Retrieve VC configuration from the cluster only of it has not been passed by the caller
	if _, ok := params[ivd.HostVcParamKey]; !ok {
		err := utils.RetrieveVcConfigSecret(params, nil, logger)

		if err != nil {
			logger.WithError(err).Errorf("Could not retrieve vsphere credential from k8s secret.")
			return nil, err
		}
		logger.Infof("DataMover: vSphere VC credential is retrieved")
	}

	// TODO: Do not initialize the data mover with the remote storage location once we move to
	// using the BackupRepositories
	var s3PETM *s3repository.ProtectedEntityTypeManager
	if !externalDataMgr {
		err := utils.RetrieveVSLFromVeleroBSLs(params, constants.DefaultS3BackupLocation, nil, logger)
		if err != nil {
			logger.WithError(err).Errorf("Could not retrieve velero default backup location.")
			return nil, err
		}
		logger.Infof("DataMover: Velero Backup Storage Location is retrieved, region=%v, bucket=%v",
			params["region"], params["bucket"])

		s3PETM, err = utils.GetS3PETMFromParamsMap(params, logger)
		if err != nil {
			logger.WithError(err).Errorf("Failed to get s3PETM from params map, region=%v, bucket=%v",
				params["region"], params["bucket"])
			return nil, err
		}
		logger.Infof("DataMover: Get s3PETM from the params map")
	}

	ivdPETM, err := utils.GetIVDPETMFromParamsMap(params, logger)
	if err != nil {
		logger.WithFields(logrus.Fields{
			"VirtualCenter": params["VirtualCenter"],
			"port":          params["port"],
		}).WithError(err).Errorf("Failed to get ivdPETM from params map.")
		return nil, err
	}
	logger.Infof("DataMover: Get ivdPETM from the params map")

	var syncMap sync.Map
	dataMover := DataMover{
		FieldLogger:         logger,
		ivdPETM:             ivdPETM,
		s3PETM:              s3PETM,
		inProgressCancelMap: &syncMap,
	}

	logger.Infof("DataMover is initialized")
	return &dataMover, nil
}

func (this *DataMover) CopyToRepo(peID astrolabe.ProtectedEntityID) (astrolabe.ProtectedEntityID, error) {
	backupRepository := builder.ForBackupRepository(constants.WithoutBackupRepository).Result()
	return this.copyToRepo(peID, backupRepository)
}

func (this *DataMover) CopyToRepoWithBackupRepository(peID astrolabe.ProtectedEntityID, backupRepository *backupdriverv1.BackupRepository) (astrolabe.ProtectedEntityID, error) {
	return this.copyToRepo(peID, backupRepository)
}

func (this *DataMover) copyToRepo(peID astrolabe.ProtectedEntityID, backupRepository *backupdriverv1.BackupRepository) (astrolabe.ProtectedEntityID, error) {
	log := this.WithField("Local PEID", peID.String())
	log.Infof("Copying the snapshot from local to remote repository")
	ctx := context.Background()
	updatedPE, err := this.ivdPETM.GetProtectedEntity(ctx, peID)
	if err != nil {
		log.WithError(err).Errorf("Failed to get ProtectedEntity")
		return astrolabe.ProtectedEntityID{}, err
	}

	log.Infof("Registering a in-progress cancel function.")
	ctx, cancelFunc := context.WithCancel(ctx)
	this.RegisterOngoingUpload(peID, cancelFunc)

	log.Debugf("Ready to call s3 PETM copy API for local PE")
	if backupRepository.Name != constants.WithoutBackupRepository {
		repoPETM, err := backuprepository.GetRepositoryFromBackupRepository(backupRepository, log)
		if err != nil {
			log.WithError(err).Errorf("Failed to get S3 repository from backup repository %s", backupRepository.Name)
			return astrolabe.ProtectedEntityID{}, err
		}
		this.s3PETM = repoPETM
	}

	var params map[string]map[string]interface{}
	s3PE, err := this.s3PETM.Copy(ctx, updatedPE, params, astrolabe.AllocateNewObject)

	log.Debugf("Return from the call of s3 PETM copy API for local PE")
	if err != nil {
		log.WithError(err).Errorf("Failed at copying to remote repository")
		return astrolabe.ProtectedEntityID{}, err
	}

	log.WithField("Remote s3PEID", s3PE.GetID().String()).Infof("Protected Entity was just copied from local to remote repository.")
	return s3PE.GetID(), nil
}

func (this *DataMover) CopyFromRepo(peID astrolabe.ProtectedEntityID, targetPEID astrolabe.ProtectedEntityID, options astrolabe.CopyCreateOptions) (astrolabe.ProtectedEntityID, error) {
	backupRepository := builder.ForBackupRepository(constants.WithoutBackupRepository).Result()
	return this.copyFromRepo(peID, targetPEID, backupRepository, options)
}

func (this *DataMover) CopyFromRepoWithBackupRepository(peID astrolabe.ProtectedEntityID, targetPEID astrolabe.ProtectedEntityID, backupRepository *backupdriverv1.BackupRepository, options astrolabe.CopyCreateOptions) (astrolabe.ProtectedEntityID, error) {
	return this.copyFromRepo(peID, targetPEID, backupRepository, options)
}

func (this *DataMover) copyFromRepo(peID astrolabe.ProtectedEntityID, targetPEID astrolabe.ProtectedEntityID, backupRepository *backupdriverv1.BackupRepository, options astrolabe.CopyCreateOptions) (astrolabe.ProtectedEntityID, error) {
	log := this.WithField("Remote PEID", peID.String())
	log.Infof("Copying the snapshot from remote repository to local. Copy options: %d", options)
	if backupRepository.Name != constants.WithoutBackupRepository {
		repoPETM, err := backuprepository.GetRepositoryFromBackupRepository(backupRepository, log)
		if err != nil {
			log.WithError(err).Errorf("Failed to get S3 repository from backup repository %s", backupRepository.Name)
			return astrolabe.ProtectedEntityID{}, err
		}
		this.s3PETM = repoPETM
	}
	ctx := context.Background()
	pe, err := this.s3PETM.GetProtectedEntity(ctx, peID)
	if err != nil {
		log.WithError(err).Errorf("Failed to get ProtectedEntity from remote PEID")
		return astrolabe.ProtectedEntityID{}, err
	}

	// Options is UpdateExistingObject. Overwrite target with the snapshot
	if options == astrolabe.UpdateExistingObject {
		// Overwrite target pe with source pe
		log.Infof("Overwriting the target PE %s with the snapshot from remote repository source PE %s.", targetPEID.String(), peID.String())
		targetPE, err := this.ivdPETM.GetProtectedEntity(ctx, targetPEID)
		if err != nil {
			log.WithError(err).Errorf("Failed to get ProtectedEntity from target PEID %s", targetPEID.String())
			return astrolabe.ProtectedEntityID{}, err
		}

		var params map[string]map[string]interface{}
		err = targetPE.Overwrite(ctx, pe, params, true)
		log.Infof("Return from the call of ivd PE overwrite API for remote PE.")
		if err != nil {
			log.WithError(err).Errorf("Failed to overwrite from remote repository.")
			return astrolabe.ProtectedEntityID{}, err
		}

		log.WithField("Local peID", targetPE.GetID().String()).Infof("Protected Entity was just overwritten by snapshot from remote repository.")
		return targetPE.GetID(), nil
	}

	log.Debugf("Ready to call ivd PETM copy API for remote PE.")
	var params map[string]map[string]interface{}
	// options should be astrolabe.AllocateNewObject
	ivdPE, err := this.ivdPETM.Copy(ctx, pe, params, options)
	log.Debugf("Return from the call of ivd PETM copy API for remote PE.")
	if err != nil {
		log.WithError(err).Errorf("Failed to copy from remote repository.")
		return astrolabe.ProtectedEntityID{}, err
	}

	log.WithField("Local peID", ivdPE.GetID().String()).Infof("Protected Entity was just copied from remote repository to local.")
	return ivdPE.GetID(), nil
}

func (this *DataMover) IsUploading(peID astrolabe.ProtectedEntityID) bool {
	log := this.WithField("PEID", peID.String())
	log.Infof("Checking if the node is uploading")
	_, ok := this.inProgressCancelMap.Load(peID)
	return ok
}

func (this *DataMover) CancelUpload(peID astrolabe.ProtectedEntityID) error {
	log := this.WithField("PEID", peID.String())
	if value, ok := this.inProgressCancelMap.Load(peID); ok {
		log.Infof("Triggering cancellation of the upload.")
		// Cast it to the cancel function, followed by invoking it.
		cancelFunc := value.(context.CancelFunc)
		cancelFunc()
		log.Infof("Triggering cancellation of the upload complete.")
		// Cant call UnregisterOngoingUpload as it picks up the lock again.
		this.inProgressCancelMap.Delete(peID)
		log.Infof("Deleted entry from the on-going cancellation map")
		return nil
	} else {
		return errors.Errorf("The pe was not found to be uploading on the node.")
	}
}

func (this *DataMover) RegisterOngoingUpload(peID astrolabe.ProtectedEntityID, cancelFunc context.CancelFunc) {
	log := this.WithField("PEID", peID.String())
	this.inProgressCancelMap.Store(peID, cancelFunc)
	log.Infof("Registered a on-going cancel function.")
}

func (this *DataMover) UnregisterOngoingUpload(peID astrolabe.ProtectedEntityID) {
	log := this.WithField("PEID", peID.String())
	if _, ok := this.inProgressCancelMap.Load(peID); !ok {
		log.Infof("The peID was unregistered previously mostly due to a triggered cancel")
	} else {
		this.inProgressCancelMap.Delete(peID)
		log.Infof("Unregistered from on-going upload map.")
	}
}
