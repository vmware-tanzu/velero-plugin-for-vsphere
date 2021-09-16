/*
 * Copyright 2019 the Astrolabe contributors
 * SPDX-License-Identifier: Apache-2.0
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package ivd

import (
	"context"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/vmware-tanzu/astrolabe/pkg/astrolabe"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/common/vsphere"
	"github.com/vmware-tanzu/astrolabe/pkg/util"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
	"github.com/vmware/govmomi/vslm"
	vslmtypes "github.com/vmware/govmomi/vslm/types"
	"github.com/vmware/virtual-disks/pkg/disklib"
	"io"
	"sync"
	"time"
)

const vsphereMajor = 6
const vSphereMinor = 7
const disklibLib64 = "/usr/lib/vmware-vix-disklib/lib64"
const vddkconfig = "vddk-config"

var configLoadLock sync.Mutex

type IVDProtectedEntityTypeManager struct {
	vcenterConfig *vsphere.VirtualCenterConfig
	vcenter       *vsphere.VirtualCenter
	vslmManager   *vsphere.VslmManager
	cnsManager    *vsphere.CnsManager
	s3Config      astrolabe.S3Config
	logger        logrus.FieldLogger
}

func NewIVDProtectedEntityTypeManager(params map[string]interface{},
	s3Config astrolabe.S3Config,
	logger logrus.FieldLogger) (astrolabe.ProtectedEntityTypeManager, error) {
	logger.Infof("Initializing IVD Protected Entity Manager")
	retVal := IVDProtectedEntityTypeManager{
		s3Config:      s3Config,
		logger:        logger,
	}
	logger.Infof("Loading new IVD Protected Entity Manager")
	err := retVal.ReloadConfig(context.Background(), params)
	if err != nil {
		logger.Errorf("Could not initialize ivd service, error: %v", err)
		logger.Warn("Saving uninitialized ivd service, will retry on service access..")
	}
	return &retVal, nil
}

func (this *IVDProtectedEntityTypeManager) GetTypeName() string {
	return "ivd"
}

func (this *IVDProtectedEntityTypeManager) GetProtectedEntity(ctx context.Context, id astrolabe.ProtectedEntityID) (astrolabe.ProtectedEntity, error) {
	err := this.CheckVcenterConnections(ctx)
	if err != nil {
		return nil, err
	}
	retIPE, err := newIVDProtectedEntity(this, id)
	if err != nil {
		return nil, err
	}
	return retIPE, nil
}

func (this *IVDProtectedEntityTypeManager) GetProtectedEntities(ctx context.Context) ([]astrolabe.ProtectedEntityID, error) {
	err := this.CheckVcenterConnections(ctx)
	if err != nil {
		return []astrolabe.ProtectedEntityID{}, err
	}
	// Kludge because of PR
	spec := vslmtypes.VslmVsoVStorageObjectQuerySpec{
		QueryField:    "createTime",
		QueryOperator: "greaterThan",
		QueryValue:    []string{"0"},
	}
	res, err := this.vslmManager.ListObjectsForSpec(ctx, []vslmtypes.VslmVsoVStorageObjectQuerySpec{spec}, 1000)
	if err != nil {
		return nil, err
	}
	retIDs := make([]astrolabe.ProtectedEntityID, len(res.Id))
	for idNum, curVSOID := range res.Id {
		retIDs[idNum] = newProtectedEntityID(curVSOID)
	}
	return retIDs, nil
}

func (this *IVDProtectedEntityTypeManager) Copy(ctx context.Context, sourcePE astrolabe.ProtectedEntity,
	params map[string]map[string]interface{}, options astrolabe.CopyCreateOptions) (astrolabe.ProtectedEntity, error) {
	sourcePEInfo, err := sourcePE.GetInfo(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "GetInfo failed")
	}
	dataReader, err := sourcePE.GetDataReader(ctx)

	if err != nil {
		return nil, errors.Wrap(err, "GetDataReader failed")
	}

	if dataReader != nil { // If there's no data we will not get a reader and no error
		defer func() {
			if err := dataReader.Close(); err != nil {
				this.logger.Errorf("The deferred data reader is closed with error, %v", err)
			}
		}()
	}

	metadataReader, err := sourcePE.GetMetadataReader(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "GetMetadataReader failed")
	}
	returnPE, err := this.copyInt(ctx, sourcePEInfo, options, dataReader, metadataReader)
	if err != nil {
		return nil, errors.Wrap(err, "copyInt failed")
	}
	return returnPE, nil
}

func (this *IVDProtectedEntityTypeManager) CopyFromInfo(ctx context.Context, peInfo astrolabe.ProtectedEntityInfo,
	params map[string]map[string]interface{}, options astrolabe.CopyCreateOptions) (astrolabe.ProtectedEntity, error) {
	return nil, nil
}

func (this *IVDProtectedEntityTypeManager) copyInt(ctx context.Context, sourcePEInfo astrolabe.ProtectedEntityInfo,
	options astrolabe.CopyCreateOptions, dataReader io.Reader, metadataReader io.Reader) (astrolabe.ProtectedEntity, error) {
	this.logger.Infof("ivd PETM copyInt called")
	if sourcePEInfo.GetID().GetPeType() != "ivd" {
		return nil, errors.New("Copy source must be an ivd")
	}
	var err error
	ourVC := false
	existsInOurVC := false
	for _, checkData := range sourcePEInfo.GetDataTransports() {
		vcenterURL, ok := checkData.GetParam("vcenter")

		if checkData.GetTransportType() == "vadp" && ok && vcenterURL == this.vcenter.Client.URL().Host {
			ourVC = true
			existsInOurVC = true
			break
		}
	}

	if ourVC {
		_, err := this.vslmManager.Retrieve(ctx, NewVimIDFromPEID(sourcePEInfo.GetID()))
		if err != nil {
			if soap.IsSoapFault(err) {
				fault := soap.ToSoapFault(err).Detail.Fault
				if _, ok := fault.(types.NotFound); ok {
					// Doesn't exist in our local system, we can't just clone it
					existsInOurVC = false
				} else {
					return nil, errors.Wrap(err, "Retrieve failed")
				}
			}
		}
	}

	this.logger.WithField("ourVC", ourVC).WithField("existsInOurVC", existsInOurVC).Debug("Ready to restore from snapshot")

	var retPE IVDProtectedEntity
	var createTask *vslm.Task

	md, err := readMetadataFromReader(ctx, metadataReader)
	if err != nil {
		return nil, err
	}

	if ourVC && existsInOurVC {
		md, err := FilterLabelsFromMetadataForVslmAPIs(md, this.vcenterConfig, this.logger)
		if err != nil {
			return nil, err
		}
		hasSnapshot := sourcePEInfo.GetID().HasSnapshot()
		if hasSnapshot {
			createTask, err = this.vslmManager.CreateDiskFromSnapshot(ctx, NewVimIDFromPEID(sourcePEInfo.GetID()), NewVimSnapshotIDFromPEID(sourcePEInfo.GetID()),
				sourcePEInfo.GetName(), nil, nil, "")
			if err != nil {
				return nil, errors.Wrap(err, "CreateDiskFromSnapshot failed")
			}
		} else {
			keepAfterDeleteVm := true
			cloneSpec := types.VslmCloneSpec{
				Name:              sourcePEInfo.GetName(),
				KeepAfterDeleteVm: &keepAfterDeleteVm,
				Metadata:          md.ExtendedMetadata,
			}
			createTask, err = this.vslmManager.Clone(ctx, NewVimIDFromPEID(sourcePEInfo.GetID()), cloneSpec)
		}

		if err != nil {
			this.logger.WithError(err).WithField("HasSnapshot", hasSnapshot).Error("Failed at creating volume from local snapshot/volume")
			return nil, err
		}

		retVal, err := createTask.WaitNonDefault(ctx, time.Hour*24, time.Second*10, true, time.Second*30)
		if err != nil {
			this.logger.WithError(err).Error("Failed at waiting for the task of creating volume")
			return nil, err
		}
		newVSO := retVal.(types.VStorageObject)
		retPE, err = newIVDProtectedEntity(this, newProtectedEntityID(newVSO.Config.Id))

		// if there is any local snasphot, we need to call updateMetadata explicitly
		// since CreateDiskFromSnapshot doesn't accept metadata as a param. The API need to be changed accordingly.
		if hasSnapshot {
			updateTask, err := this.vslmManager.UpdateMetadata(ctx, newVSO.Config.Id, md.ExtendedMetadata, []string{})
			if err != nil {
				this.logger.WithError(err).Error("Failed at calling UpdateMetadata")
				return nil, err
			}
			_, err = updateTask.WaitNonDefault(ctx, time.Hour*24, time.Second*10, true, time.Second*30)
			if err != nil {
				this.logger.WithError(err).Error("Failed at waiting for the UpdateMetadata task")
				return nil, err
			}
		}
	} else {
		// To enable cross-cluster restore, need to filter out the cns specific labels, i.e. prefix: cns, in md
		md = FilterLabelsFromMetadataForCnsAPIs(md, "cns", this.logger)

		this.logger.Infof("Ready to provision a new volume with the source metadata: %v", md)
		volumeVimID, err := CreateCnsVolumeInCluster(ctx, this.vcenterConfig, this.vcenter.Client, this.cnsManager, md, this.logger)
		if err != nil {
			return nil, errors.Wrap(err, "CreateDisk failed")
		}
		retPE, err = newIVDProtectedEntity(this, newProtectedEntityID(volumeVimID))
		if err != nil {
			return nil, errors.Wrap(err, "CreateDisk failed")
		}
		_, err = retPE.copy(ctx, dataReader, md)
		if err != nil {
			this.logger.Errorf("Failed to copy data from data source to newly-provisioned IVD protected entity")
			return nil, errors.Wrapf(err, "Failed to copy data from data source to newly-provisioned IVD protected entity")
		}
		this.logger.WithField("volumeId", volumeVimID.Id).WithField("volumeName", md.VirtualStorageObject.Config.Name).Info("Copied snapshot data to newly-provisioned IVD protected entity")
	}
	return retPE, nil
}

func (this *IVDProtectedEntityTypeManager) getDataTransports(id astrolabe.ProtectedEntityID) ([]astrolabe.DataTransport,
	[]astrolabe.DataTransport,
	[]astrolabe.DataTransport, error) {
	vadpParams := make(map[string]string)
	vadpParams["id"] = id.GetID()
	if id.GetSnapshotID().String() != "" {
		vadpParams["snapshotID"] = id.GetSnapshotID().String()
	}
	vadpParams["vcenter"] = this.vcenterConfig.Host

	dataS3Transport, err := astrolabe.NewS3DataTransportForPEID(id, this.s3Config)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "Could not create S3 data transport")
	}

	data := []astrolabe.DataTransport{
		astrolabe.NewDataTransport("vadp", vadpParams),
		dataS3Transport,
	}

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

func (this *IVDProtectedEntityTypeManager) CheckVcenterConnections(ctx context.Context) error {
	this.logger.Infof("Checking vCenter connections...")
	if this.vcenter == nil || this.vslmManager == nil || this.cnsManager == nil {
		err := this.ReloadConfig(ctx, make(map[string]interface{}))
		if err != nil {
			this.logger.Errorf("The vCenter is disconnected, error: %v", err)
			return err
		}
	}
	this.logger.Infof("vCenter connected, proceeding with operation")
	return nil
}

func (this *IVDProtectedEntityTypeManager) ReloadConfig(ctx context.Context, params map[string]interface{}) error {
	configLoadLock.Lock()
	defer configLoadLock.Unlock()
	this.logger.Debug("Started Load Config of IVD Protected Entity Manager")
	var newVcConfig *vsphere.VirtualCenterConfig
	var err error
	if len(params) == 0  {
		// vc operation path, for example createSnapshot.
		// Used to verify if the connections are still up.
		// Will be using the last known vcenter config for the checks.
		newVcConfig = this.vcenterConfig
	} else {
		// The params are possibly updated, this is the password rotation path.
		newVcConfig, err = vsphere.GetVirtualCenterConfigFromParams(params, this.logger)
		if err != nil {
			this.logger.Errorf("Failed to populate VirtualCenterConfig during reload %v", err)
			return err
		}
	}
	configChanged := vsphere.CheckIfVirtualCenterConfigChanged(this.vcenterConfig, newVcConfig)
	if !configChanged {
		this.logger.Debug("No VirtualCenterConfig change detected during periodic reload.")
		// Check if the connections are initialized.
		if this.vcenter != nil {
			this.logger.Debug("The vcenter connections are alive, no reload necessary.")
			// the vcenter connection is initialized, nothing to do
			return nil
		}
	} else {
		this.logger.Infof("Detected VirtualCenterConfig change during periodic reload.")
		// Detected config change.
		this.vcenterConfig = newVcConfig
		if this.vcenter != nil {
			// Disconnecting older vc instance.
			err = this.vcenter.Disconnect(ctx)
			if err != nil {
				this.logger.Errorf("Failed to disconnect older vcenter instance.")
				return err
			}
		}
	}
	// continue to establish the connection.
	reloadedVc, vslmClient, cnsClient, err := vsphere.GetVirtualCenter(ctx, newVcConfig, this.logger)
	if err != nil {
		return err
	}
	this.vcenter = reloadedVc
	if this.vslmManager == nil {
		this.logger.Infof("Initializing VSLM Manager")
		this.vslmManager, err = vsphere.GetVslmManager(ctx, this.vcenter, vslmClient, this.logger)
		if err != nil {
			this.logger.Errorf("failed to initialize vslm manager. err=%v", err)
			return errors.Wrapf(err, "Failed to initialize vslm manager")
		}
	} else {
		this.logger.Infof("Resetting VSLM Manager")
		this.vslmManager.ResetManager(reloadedVc, vslmClient)
	}
	if this.cnsManager == nil {
		this.logger.Infof("Initializing CNS Manager")
		this.cnsManager, err = vsphere.GetCnsManager(ctx, this.vcenter, cnsClient, this.logger)
		if err != nil {
			this.logger.Errorf("failed to initialize cns manager. err=%v", err)
			return errors.Wrapf(err, "Failed to initialize cns manager")
		}
	} else {
		this.logger.Infof("Resetting CNS Manager")
		this.cnsManager.ResetManager(reloadedVc, cnsClient)
	}

	// check whether customized vddk config is provided.
	// currently only support user to modify vixdisklib nfc log level and transport log level
	path := ""
	if _, ok := params[vddkconfig]; !ok {
		this.logger.Info("No customized vddk log level provided, set vddk log level as default")
	} else {
		this.logger.Infof("Customized vddk config provided: %v", params[vddkconfig])
		vddkConfig := params[vddkconfig].(map[string]string)
		path, err = util.CreateConfigFile(vddkConfig, this.logger)
		if err != nil {
			this.logger.Error("Failed to create config file for vddk. Cannot proceed to init vddk lib")
			return errors.Wrap(err, "Failed to create config file")
		}
	}
	err = disklib.InitEx(vsphereMajor, vSphereMinor, disklibLib64, path)
	if err != nil {
		return errors.Wrap(err, "Could not initialize VDDK during config reload")
	}
	if path != "" {
		err = util.DeleteConfigFile(path, this.logger)
		if err != nil {
			return errors.Wrap(err, "Failed to delete config file")
		}
	}
	this.logger.Infof("Initialized VDDK")
	this.logger.Infof("Load Config of IVD Protected Entity Manager completed successfully")
	return nil
}
