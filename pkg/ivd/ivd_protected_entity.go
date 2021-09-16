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
	"bufio"
	"bytes"
	"fmt"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/vmware-tanzu/astrolabe/pkg/astrolabe"
	"github.com/vmware-tanzu/astrolabe/pkg/util"
	"github.com/vmware/govmomi/vim25/soap"
	vim "github.com/vmware/govmomi/vim25/types"
	"github.com/vmware/govmomi/vim25/xml"
	"github.com/vmware/virtual-disks/pkg/disklib"
	"github.com/vmware/virtual-disks/pkg/virtual_disks"
	"io"
	"io/ioutil"
	"k8s.io/apimachinery/pkg/util/wait"
	"strings"

	"context"
	"github.com/vmware/govmomi/vslm"
	vslmtypes "github.com/vmware/govmomi/vslm/types"
	"time"
)

type IVDProtectedEntity struct {
	ipetm    *IVDProtectedEntityTypeManager
	id       astrolabe.ProtectedEntityID
	data     []astrolabe.DataTransport
	metadata []astrolabe.DataTransport
	combined []astrolabe.DataTransport
	logger   logrus.FieldLogger
}

type metadata struct {
	VirtualStorageObject vim.VStorageObject         `xml:"virtualStorageObject"`
	Datastore            vim.ManagedObjectReference `xml:"datastore"`
	ExtendedMetadata     []vim.KeyValue             `xml:"extendedMetadata"`
}

func (this IVDProtectedEntity) GetDataReader(ctx context.Context) (io.ReadCloser, error) {
	this.logger.Infof("GetDataReader called on IVD Protected Entity, %v", this.id.String())
	err := this.ipetm.CheckVcenterConnections(ctx)
	if err != nil {
		return nil, err
	}
	diskConnectParam, err := this.getDiskConnectionParams(ctx, true)
	if err != nil {
		return nil, err
	}

	diskReader, vErr := virtual_disks.Open(diskConnectParam, this.logger)
	if vErr != nil {
		if vErr.VixErrorCode() == 20005 {
			// Try the end access once
			disklib.EndAccess(diskConnectParam)
			diskReader, vErr = virtual_disks.Open(diskConnectParam, this.logger)
		}
		if vErr != nil {
			return nil, errors.New(fmt.Sprintf(vErr.Error()+" with error code: %d", vErr.VixErrorCode()))
		}
	}

	return diskReader, nil
}

func (this IVDProtectedEntity) copy(ctx context.Context, dataReader io.Reader,
	metadata metadata) (int64, error) {
	// TODO - restore metadata
	dataWriter, err := this.getDataWriter(ctx)
	if dataWriter != nil {
		defer func() {
			if err := dataWriter.Close(); err != nil {
				this.logger.Errorf("The deferred data writer is closed with error, %v", err)
			}
		}()
	}

	if err != nil {
		return 0, err
	}

	bufferedWriter := bufio.NewWriterSize(dataWriter, 1024*1024)
	buf := make([]byte, 1024*1024)
	bytesWritten, err := io.CopyBuffer(bufferedWriter, dataReader, buf) // TODO - add a copy routine that we can interrupt via context
	if err != nil {
		err = errors.Wrapf(err, "Failed in CopyBuffer, bytes written = %d", bytesWritten)
	}
	return bytesWritten, err
}

func (this IVDProtectedEntity) getDataWriter(ctx context.Context) (io.WriteCloser, error) {
	diskConnectParam, err := this.getDiskConnectionParams(ctx, false)
	if err != nil {
		return nil, err
	}

	diskWriter, vErr := virtual_disks.Open(diskConnectParam, this.logger)
	if vErr != nil {
		return nil, errors.New(fmt.Sprintf(vErr.Error()+" with error code: %d", vErr.VixErrorCode()))
	}

	return diskWriter, nil
}

func (this IVDProtectedEntity) getDiskConnectionParams(ctx context.Context, readOnly bool) (disklib.ConnectParams, error) {
	vc := this.ipetm.vcenter
	_, _, err := vc.Connect(ctx)
	if err != nil {
		this.logger.Errorf("Failed to connect to VC")
		return disklib.ConnectParams{}, err
	}
	url := vc.Client.Client.URL()
	serverName := url.Hostname()
	userName := vc.Config.Username
	password := vc.Config.Password
	fcdId := this.id.GetID()

	vso, err := this.ipetm.vslmManager.Retrieve(ctx, NewVimIDFromPEID(this.id))
	if err != nil {
		return disklib.ConnectParams{}, err
	}
	datastore := vso.Config.Backing.GetBaseConfigInfoBackingInfo().Datastore.String()
	datastore = strings.TrimPrefix(datastore, "Datastore:")

	fcdssid := ""
	if this.id.HasSnapshot() {
		fcdssid = this.id.GetSnapshotID().String()
	}
	path := ""
	var flags uint32
	if readOnly {
		flags = disklib.VIXDISKLIB_FLAG_OPEN_COMPRESSION_SKIPZ | disklib.VIXDISKLIB_FLAG_OPEN_READ_ONLY
	} else {
		flags = disklib.VIXDISKLIB_FLAG_OPEN_UNBUFFERED
	}
	transportMode := "nbd"
	thumbPrint, err := disklib.GetThumbPrintForURL(*url)
	if err != nil {
		this.logger.Errorf("Failed to get the thumb print for the URL, %s", url.String())
		return disklib.ConnectParams{}, err
	}

	params := disklib.NewConnectParams("",
		serverName,
		thumbPrint,
		userName,
		password,
		fcdId,
		datastore,
		fcdssid,
		"",
		"vm-example",
		path,
		flags,
		readOnly,
		transportMode)

	return params, nil
}

func (this IVDProtectedEntity) GetMetadataReader(ctx context.Context) (io.ReadCloser, error) {
	this.logger.Infof("GetMetadataReader called on IVD Protected Entity, %v", this.id.String())
	err := this.ipetm.CheckVcenterConnections(ctx)
	if err != nil {
		return nil, err
	}
	infoBuf, err := this.getMetadataBuf(ctx)
	if err != nil {
		return nil, err
	}

	return ioutil.NopCloser(bytes.NewReader(infoBuf)), nil
}

func (this IVDProtectedEntity) getMetadataBuf(ctx context.Context) ([]byte, error) {
	md, err := this.getMetadata(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "Retrieve failed")
	}
	retBuf, err := xml.MarshalIndent(md, "  ", "    ")
	if err != nil {
		return nil, err
	}
	return retBuf, nil
}

func (this IVDProtectedEntity) getMetadata(ctx context.Context) (metadata, error) {
	vsoID := vim.ID{
		Id: this.id.GetID(),
	}
	vso, err := this.ipetm.vslmManager.Retrieve(ctx, vsoID)
	if err != nil {
		return metadata{}, err
	}
	datastore := vso.Config.BaseConfigInfo.GetBaseConfigInfo().Backing.GetBaseConfigInfoBackingInfo().Datastore
	var ssID *vim.ID = nil
	if this.id.HasSnapshot() {

		ssID = &vim.ID{
			Id: this.id.GetSnapshotID().GetID(),
		}
	}
	extendedMetadata, err := this.ipetm.vslmManager.RetrieveMetadata(ctx, vsoID, ssID, "")

	retVal := metadata{
		VirtualStorageObject: *vso,
		Datastore:            datastore,
		ExtendedMetadata:     extendedMetadata,
	}
	return retVal, nil
}

func readMetadataFromReader(ctx context.Context, metadataReader io.Reader) (metadata, error) {
	mdBuf, err := ioutil.ReadAll(metadataReader) // TODO - limit this so it can't run us out of memory here
	if err != nil && err != io.EOF {
		return metadata{}, errors.Wrap(err, "ReadAll failed")
	}
	return readMetadataFromBuf(ctx, mdBuf)
}

func readMetadataFromBuf(ctx context.Context, buf []byte) (metadata, error) {
	var retVal = metadata{}
	err := xml.Unmarshal(buf, &retVal)
	return retVal, err
}

func newProtectedEntityID(id vim.ID) astrolabe.ProtectedEntityID {
	return astrolabe.NewProtectedEntityID("ivd", id.Id)
}

func newProtectedEntityIDWithSnapshotID(id vim.ID, snapshotID astrolabe.ProtectedEntitySnapshotID) astrolabe.ProtectedEntityID {
	return astrolabe.NewProtectedEntityIDWithSnapshotID("ivd", id.Id, snapshotID)
}

func newIVDProtectedEntity(ipetm *IVDProtectedEntityTypeManager, id astrolabe.ProtectedEntityID) (IVDProtectedEntity, error) {
	data, metadata, combined, err := ipetm.getDataTransports(id)
	if err != nil {
		return IVDProtectedEntity{}, err
	}
	newIPE := IVDProtectedEntity{
		ipetm:    ipetm,
		id:       id,
		data:     data,
		metadata: metadata,
		combined: combined,
		logger:   ipetm.logger,
	}
	return newIPE, nil
}
func (this IVDProtectedEntity) GetInfo(ctx context.Context) (astrolabe.ProtectedEntityInfo, error) {
	this.logger.Infof("GetInfo called on IVD Protected Entity, %v", this.id.String())
	err := this.ipetm.CheckVcenterConnections(ctx)
	if err != nil {
		return nil, err
	}
	vsoID := vim.ID{
		Id: this.id.GetID(),
	}
	vso, err := this.ipetm.vslmManager.Retrieve(ctx, vsoID)
	if err != nil {
		return nil, errors.Wrap(err, "Retrieve failed")
	}

	retVal := astrolabe.NewProtectedEntityInfo(
		this.id,
		vso.Config.Name,
		vso.Config.CapacityInMB * 1024 * 1024,
		this.data,
		this.metadata,
		this.combined,
		[]astrolabe.ProtectedEntityID{})
	return retVal, nil
}

func (this IVDProtectedEntity) GetCombinedInfo(ctx context.Context) ([]astrolabe.ProtectedEntityInfo, error) {
	ivdIPE, err := this.GetInfo(ctx)
	if err != nil {
		return nil, err
	}
	return []astrolabe.ProtectedEntityInfo{ivdIPE}, nil
}

const waitTime = 3600 * time.Second

/*
 * Snapshot APIs
 */
func (this IVDProtectedEntity) Snapshot(ctx context.Context, params map[string]map[string]interface{}) (astrolabe.ProtectedEntitySnapshotID, error) {
	this.logger.Infof("CreateSnapshot called on IVD Protected Entity, %v", this.id.String())
	var retVal astrolabe.ProtectedEntitySnapshotID
	retryInterval := time.Second
	retryCount := 0
	retrieveSnapDetailsErr := 0
	err := wait.PollImmediate(retryInterval, time.Hour, func() (bool, error) {
		err := this.ipetm.CheckVcenterConnections(ctx)
		if err != nil {
			this.logger.Errorf("The vCenter is disconnected during snapshot operation, error: %v, retrying..", err)
			return false, nil
		}
		this.logger.Infof("Retrying CreateSnapshot on IVD Protected Entity, %v, for one hour at the maximum, Current retry count: %d", this.GetID().String(), retryCount)
		var vslmTask *vslm.Task
		vslmTask, err = this.ipetm.vslmManager.CreateSnapshot(ctx, NewVimIDFromPEID(this.GetID()), "AstrolabeSnapshot")
		if err != nil {
			return false, errors.Wrapf(err, "Failed to create a task for the CreateSnapshot invocation on IVD Protected Entity, %v", this.id.String())
		}
		this.logger.Infof("Retrieved VSLM task %s to track CreateSnapshot on IVD %s", vslmTask.Value, this.GetID().String())
		retryCount++
		start := time.Now()
		ivdSnapshotIDAny, err := vslmTask.Wait(ctx, waitTime)
		this.logger.Infof("Waited for %s to retrieve Task %s status.", time.Now().Sub(start), vslmTask.Value)
		if err != nil {
			if soap.IsVimFault(err) {
				_, ok := soap.ToVimFault(err).(*vim.InvalidState)
				if ok {
					this.logger.WithError(err).Errorf("There is some operation, other than this CreateSnapshot invocation, on the VM attached still being protected by its VM state. Will retry in %v second(s)", retryInterval)
					return false, nil
				}
				_, ok = soap.ToVimFault(err).(*vslmtypes.VslmSyncFault)
				if ok {
					this.logger.WithError(err).Errorf("CreateSnapshot failed with VslmSyncFault possibly due to race between concurrent DeleteSnapshot invocation. Will retry in %v second(s)", retryInterval)
					return false, nil
				}
				_, ok = soap.ToVimFault(err).(*vim.NotFound)
				if ok {
					this.logger.WithError(err).Errorf("CreateSnapshot failed with NotFound. Will retry in %v second(s)", retryInterval)
					return false, nil
				}
			}
			if util.IsConnectionResetError(err) {
				this.logger.WithError(err).Errorf("Network issue: connection reset by peer. Will retry in %v second(s)", retryInterval)
				return false, nil
			}
			this.logger.Errorf("Error on CreateSnapshot: %+s", err.Error())
			return false, errors.Wrapf(err, "Failed at waiting for the CreateSnapshot invocation on IVD Protected Entity, %v, Retry-Count: %d", this.id.String(), retryCount)
		}
		ivdSnapshotID := ivdSnapshotIDAny.(vim.ID)
		this.logger.Infof("A new snapshot, %v, was created on IVD Protected Entity, %v, Retry-Count: %d, RetrieveSnapshotErr: %d", ivdSnapshotID.Id, this.GetID().String(), retryCount, retrieveSnapDetailsErr)

		// Will try RetrieveSnapshotDetail right after the completion of CreateSnapshot to make sure there is no impact from race condition
		_, err = this.ipetm.vslmManager.RetrieveSnapshotDetails(ctx, NewVimIDFromPEID(this.GetID()), ivdSnapshotID)
		if err != nil {
			retrieveSnapDetailsErr++
			if soap.IsSoapFault(err) {
				faultMsg := soap.ToSoapFault(err).String
				if strings.Contains(faultMsg, "A specified parameter was not correct: snapshotId") {
					this.logger.WithError(err).Error("Unexpected InvalidArgument SOAP fault due to the known race condition. Will retry")
					return false, nil
				}
			}
			this.logger.WithError(err).Warnf("Failed at retrieving the snapshot details post the" +
				" completion of CreateSnapshot on %s, proceeding to use Snapshot %s anyways", this.id.String(), ivdSnapshotID.Id)
		} else {
			this.logger.Infof("The retrieval of the newly created snapshot, %s on IVD %s, " +
				"is completed successfully, Retry-Count: %d, RetrieveSnapshotErr: %d",
				ivdSnapshotID.Id, this.id.String(), retryCount, retrieveSnapDetailsErr)
		}
		retVal = astrolabe.NewProtectedEntitySnapshotID(ivdSnapshotID.Id)
		return true, nil
	})

	if err != nil {
		return astrolabe.ProtectedEntitySnapshotID{}, err
	}
	this.logger.Infof("CreateSnapshot completed on IVD Protected Entity, %v, with the new snapshot, %v, being created", this.id.String(), retVal.String())
	return retVal, nil
}

func (this IVDProtectedEntity) ListSnapshots(ctx context.Context) ([]astrolabe.ProtectedEntitySnapshotID, error) {
	peSnapshotIDs := []astrolabe.ProtectedEntitySnapshotID{}
	this.logger.Infof("ListSnapshots called on IVD Protected Entity, %v", this.id.String())
	err := this.ipetm.CheckVcenterConnections(ctx)
	if err != nil {
		return peSnapshotIDs, err
	}
	snapshotInfo, err := this.ipetm.vslmManager.RetrieveSnapshotInfo(ctx, NewVimIDFromPEID(this.GetID()))
	if err != nil {
		return nil, errors.Wrap(err, "RetrieveSnapshotInfo failed")
	}
	for _, curSnapshotInfo := range snapshotInfo {
		peSnapshotIDs = append(peSnapshotIDs, astrolabe.NewProtectedEntitySnapshotID(curSnapshotInfo.Id.Id))
	}
	this.logger.Infof("Retrieved %d snapshots for pe-id: %s, snapshots= %v", len(peSnapshotIDs), this.GetID().String(), peSnapshotIDs)
	return peSnapshotIDs, nil
}
func (this IVDProtectedEntity) DeleteSnapshot(ctx context.Context, snapshotToDelete astrolabe.ProtectedEntitySnapshotID, params map[string]map[string]interface{}) (bool, error) {
	this.logger.Infof("DeleteSnapshot called on IVD Protected Entity, %v, with input arg, %v", this.GetID().String(), snapshotToDelete.String())
	retryCount := 0
	err := wait.PollImmediate(time.Second, time.Hour, func() (bool, error) {
		err := this.ipetm.CheckVcenterConnections(ctx)
		if err != nil {
			this.logger.Errorf("The vCenter is disconnected during DeleteSnapshot operation, error: %v, retrying..", err)
			return false, nil
		}
		this.logger.Debugf("Retrying DeleteSnapshot on IVD Protected Entity, %v, for one hour at the maximum", this.GetID().String())
		vslmTask, err := this.ipetm.vslmManager.DeleteSnapshot(ctx, NewVimIDFromPEID(this.GetID()), NewVimSnapshotIDFromPESnapshotID(snapshotToDelete))
		if err != nil {
			// If the FCD is non-existent, Pandora immediately returns NotFound instead of creating a task.
			if soap.IsSoapFault(err) {
				soapFault := soap.ToSoapFault(err)
				receivedFault := soapFault.Detail.Fault
				_, ok := receivedFault.(vim.NotFound)
				if ok {
					this.logger.Warnf("The FCD id %s was not found in VC during deletion, assuming success, received error: %+v", this.GetID().GetID(), err)
					return true, nil
				}
			}
			return false, errors.Wrapf(err, "Failed to create a task for the DeleteSnapshot invocation on IVD Protected Entity, %v, with input arg, %v", this.GetID().String(), snapshotToDelete.String())
		}
		this.logger.Infof("Retrieved VSLM task %s to track DeleteSnapshot on IVD %s", vslmTask.Value, this.GetID().String())
		retryCount++
		start := time.Now()
		_, err = vslmTask.Wait(ctx, waitTime)
		this.logger.Infof("Waited for %s to retrieve Task %s status.", time.Now().Sub(start), vslmTask.Value)
		if err != nil {
			if soap.IsVimFault(err) {
				switch soap.ToVimFault(err).(type) {
				case *vim.InvalidArgument:
					this.logger.WithError(err).Infof("Disk doesn't have given snapshot due to the snapshot stamp was removed in the previous DeleteSnapshot operation which failed with InvalidState fault. And it will be resolved by the next snapshot operation on the same VM. Will NOT retry")
					return true, nil
				case *vim.NotFound:
					this.logger.WithError(err).Infof("There is a temporary catalog mismatch due to a race condition with one another concurrent DeleteSnapshot operation. And it will be resolved by the next consolidateDisks operation on the same VM. Will NOT retry")
					return true, nil
				case *vim.InvalidState:
					this.logger.WithError(err).Error("There is some operation, other than this DeleteSnapshot invocation, on the same VM still being protected by its VM state. Will retry")
					return false, nil
				case *vim.TaskInProgress:
					this.logger.WithError(err).Error("There is some other InProgress operation on the same VM. Will retry")
					return false, nil
				case *vim.FileLocked:
					this.logger.WithError(err).Error("An error occurred while consolidating disks: Failed to lock the file. Will retry")
					return false, nil
				}
			}
			if util.IsConnectionResetError(err) {
				this.logger.WithError(err).Error("Network issue: connection reset by peer. Will retry")
				return false, nil
			}
			return false, errors.Wrapf(err, "Failed at waiting for the DeleteSnapshot invocation on IVD Protected Entity, %v, with input arg, %v", this.GetID().String(), snapshotToDelete.String())
		}
		return true, nil
	})

	if err != nil {
		return false, err
	}
	this.logger.Infof("DeleteSnapshot completed on IVD Protected Entity, %s, with input arg, %s, Retry-Count: %d", this.GetID().String(), snapshotToDelete.String(), retryCount)
	return true, nil
}

func (this IVDProtectedEntity) GetInfoForSnapshot(ctx context.Context, snapshotID astrolabe.ProtectedEntitySnapshotID) (*astrolabe.ProtectedEntityInfo, error) {
	return nil, nil
}

func (this IVDProtectedEntity) GetComponents(ctx context.Context) ([]astrolabe.ProtectedEntity, error) {
	return make([]astrolabe.ProtectedEntity, 0), nil
}

func (this IVDProtectedEntity) GetID() astrolabe.ProtectedEntityID {
	return this.id
}

func (this IVDProtectedEntity) Overwrite(ctx context.Context, sourcePE astrolabe.ProtectedEntity, params map[string]map[string]interface{},
	overwriteComponents bool) error {
	this.logger.Infof("Overwriting current IVD PE %s with the source PE %s", this.GetID().String(), sourcePE.GetID().String())
	// overwriteComponents is ignored because we have no components
	if sourcePE.GetID().GetPeType() != "ivd" {
		return errors.New("Overwrite source must be an ivd")
	}
	err := this.ipetm.CheckVcenterConnections(ctx)
	if err != nil {
		return err
	}
	// TODO - verify that our size is >= sourcePE size
	metadataReader, err := sourcePE.GetMetadataReader(ctx)
	if err != nil {
		return errors.Wrap(err, "Could not retrieve metadata reader")
	}

	dataReader, err := sourcePE.GetDataReader(ctx)
	if err != nil {
		return errors.Wrap(err, "Could not retrieve data reader")
	}

	md, err := readMetadataFromReader(ctx, metadataReader)
	// To enable cross-cluster restore, need to filter out the cns specific labels, i.e. prefix: cns, in md
	md = FilterLabelsFromMetadataForCnsAPIs(md, "cns", this.logger)

	_, err = this.copy(ctx, dataReader, md)
	if err != nil {
		return errors.Wrapf(err, "Data copy failed from PE %s, to PE %s", this.GetID().String(), sourcePE.GetID().String())
	}
	this.logger.Infof("Successfully Overwrote current IVD PE %s with the source PE %s", this.GetID().String(), sourcePE.GetID().String())
	return nil
}

func NewIDFromString(idStr string) vim.ID {
	return vim.ID{
		Id: idStr,
	}
}

func NewVimIDFromPEID(peid astrolabe.ProtectedEntityID) vim.ID {
	if peid.GetPeType() == "ivd" {
		return vim.ID{
			Id: peid.GetID(),
		}
	} else {
		return vim.ID{}
	}
}

func NewVimSnapshotIDFromPEID(peid astrolabe.ProtectedEntityID) vim.ID {
	if peid.HasSnapshot() {
		return vim.ID{
			Id: peid.GetSnapshotID().GetID(),
		}
	} else {
		return vim.ID{}
	}
}

func NewVimSnapshotIDFromPESnapshotID(peSnapshotID astrolabe.ProtectedEntitySnapshotID) vim.ID {
	return vim.ID{
		Id: peSnapshotID.GetID(),
	}
}
