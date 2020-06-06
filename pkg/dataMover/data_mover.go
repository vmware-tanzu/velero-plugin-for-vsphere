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
	"github.com/sirupsen/logrus"
	"github.com/vmware-tanzu/astrolabe/pkg/astrolabe"
	"github.com/vmware-tanzu/astrolabe/pkg/ivd"
	"github.com/vmware-tanzu/astrolabe/pkg/s3repository"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/utils"
	"sync"
)

type DataMover struct {
	logrus.FieldLogger
	ivdPETM             *ivd.IVDProtectedEntityTypeManager
	s3PETM              *s3repository.ProtectedEntityTypeManager
	mutex               *sync.Mutex
	inProgressCancelMap map[astrolabe.ProtectedEntityID]context.CancelFunc
}

func NewDataMoverFromCluster(logger logrus.FieldLogger) (*DataMover, error) {
	params := make(map[string]interface{})
	err := utils.RetrieveVcConfigSecret(params, logger)

	if err != nil {
		logger.WithError(err).Errorf("Could not retrieve vsphere credential from k8s secret.")
		return nil, err
	}
	logger.Infof("DataMover: vSphere VC credential is retrieved")

	err = utils.RetrieveVSLFromVeleroBSLs(params, logger)
	if err != nil {
		logger.WithError(err).Errorf("Could not retrieve velero default backup location.")
		return nil, err
	}
	logger.Infof("DataMover: Velero Backup Storage Location is retrieved, region=%v, bucket=%v",
		params["region"], params["bucket"])

	s3PETM, err := utils.GetS3PETMFromParamsMap(params, logger)
	if err != nil {
		logger.WithError(err).Errorf("Failed to get s3PETM from params map, region=%v, bucket=%v",
			params["region"], params["bucket"])
		return nil, err
	}
	logger.Infof("DataMover: Get s3PETM from the params map")

	ivdPETM, err := utils.GetIVDPETMFromParamsMap(params, logger)
	if err != nil {
		logger.WithFields(logrus.Fields{
			"region":        params["region"],
			"bucket":        params["bucket"],
			"VirtualCenter": params["VirtualCenter"],
			"port":          params["port"],
		}).WithError(err).Errorf("Failed to get ivdPETM from params map.")
		return nil, err
	}
	logger.Infof("DataMover: Get ivdPETM from the params map")

	var mutex sync.Mutex

	dataMover := DataMover{
		FieldLogger:         logger,
		ivdPETM:             ivdPETM,
		s3PETM:              s3PETM,
		mutex:               &mutex,
		inProgressCancelMap: make(map[astrolabe.ProtectedEntityID]context.CancelFunc),
	}

	logger.Infof("DataMover is initialized")
	return &dataMover, nil
}

func (this *DataMover) CopyToRepo(peID astrolabe.ProtectedEntityID) (astrolabe.ProtectedEntityID, error) {
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
	s3PE, err := this.s3PETM.Copy(ctx, updatedPE, astrolabe.AllocateNewObject)
	log.Debugf("Return from the call of s3 PETM copy API for local PE")
	if err != nil {
		log.WithError(err).Errorf("Failed at copying to remote repository")
		return astrolabe.ProtectedEntityID{}, err
	}

	log.WithField("Remote s3PEID", s3PE.GetID().String()).Infof("Protected Entity was just copied from local to remote repository.")
	return s3PE.GetID(), nil
}

func (this *DataMover) CopyFromRepo(peID astrolabe.ProtectedEntityID) (astrolabe.ProtectedEntityID, error) {
	log := this.WithField("Remote PEID", peID.String())
	log.Infof("Copying the snapshot from remote repository to local.")
	ctx := context.Background()
	pe, err := this.s3PETM.GetProtectedEntity(ctx, peID)
	if err != nil {
		log.WithError(err).Errorf("Failed to get ProtectedEntity from remote PEID")
		return astrolabe.ProtectedEntityID{}, err
	}

	log.Debugf("Ready to call ivd PETM copy API for remote PE.")
	ivdPE, err := this.ivdPETM.Copy(ctx, pe, astrolabe.AllocateNewObject)
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
	this.mutex.Lock()
	defer this.mutex.Unlock()
	log.Infof("Checking if the node is uploading")
	_, ok := this.inProgressCancelMap[peID]
	return ok
}

func (this *DataMover) CancelUpload(peID astrolabe.ProtectedEntityID) {
	log := this.WithField("PEID", peID.String())
	this.mutex.Lock()
	defer this.mutex.Unlock()
	if cancelFunc, ok := this.inProgressCancelMap[peID]; ok {
		log.Infof("Triggering cancellation of the upload.")
		cancelFunc()
		log.Infof("Triggering cancellation of the upload complete.")
		// Cant call UnregisterOngoingUpload as it picks up the lock again.
		delete(this.inProgressCancelMap, peID)
		log.Infof("Deleted entry %v from the on-going cancellation map", peID)
	} else {
		log.Errorf("The pe was not found to be uploading on the node.")
	}
}

func (this *DataMover) RegisterOngoingUpload(peID astrolabe.ProtectedEntityID, cancelFunc context.CancelFunc) {
	log := this.WithField("PEID", peID.String())
	this.mutex.Lock()
	defer this.mutex.Unlock()
	this.inProgressCancelMap[peID] = cancelFunc
	log.Infof("Registered a on-going cancel function.")
}

func (this *DataMover) UnregisterOngoingUpload(peID astrolabe.ProtectedEntityID) {
	log := this.WithField("PEID", peID.String())
	this.mutex.Lock()
	defer this.mutex.Unlock()
	if _, ok := this.inProgressCancelMap[peID]; !ok {
		log.Infof("The peID was unregistered previously mostly due to a triggered cancel")
	} else {
		delete(this.inProgressCancelMap, peID)
		log.Infof("Unregistered from on-going upload map.")
	}
}
