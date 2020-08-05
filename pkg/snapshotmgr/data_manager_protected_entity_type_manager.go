package snapshotmgr

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
)

import "github.com/vmware-tanzu/astrolabe/pkg/astrolabe"

type DataManagerProtectedEntityTypeManager struct {
	astrolabe.ProtectedEntityTypeManager
	snapshotMgr *SnapshotManager
	upload bool
}

/*
Creates a DataManagerProtectedEntityTypeManager that wrappers the sourcePETM (and by extension, the PEs managed by the
source PETM).  Data Manager currently has upload and download CRs.  If upload is set, Copy, CopyFromInfo and Overwrite will generate
Upload CRs to the source PETM type.  If not set, Download CRs will be generated
 */
func NewDataManagerProtectedEntityTypeManager(sourcePETM astrolabe.ProtectedEntityTypeManager,
	snapshotMGR *SnapshotManager, upload bool) DataManagerProtectedEntityTypeManager {
	return DataManagerProtectedEntityTypeManager{
		ProtectedEntityTypeManager: sourcePETM,
		snapshotMgr:                snapshotMGR,
		upload: upload,
	}
}

func (this DataManagerProtectedEntityTypeManager) GetProtectedEntity(ctx context.Context, id astrolabe.ProtectedEntityID) (astrolabe.ProtectedEntity, error) {
	basePE, err := this.GetProtectedEntity(ctx, id)
	if err != nil {
		return nil, errors.Wrapf(err, "GetProtectedEntity from base PETM failed for PEID %s", id.String())
	}
	return newDataManagerProtectedEntity(basePE, &this), nil
}

func (this DataManagerProtectedEntityTypeManager) Copy(ctx context.Context, pe astrolabe.ProtectedEntity,
	params map[string]map[string]interface{}, options astrolabe.CopyCreateOptions) (astrolabe.ProtectedEntity, error) {
	fmt.Println("Copy called")
	// TODO - Implement this
	if this.upload {

	} else {
		//this.snapshotMgr.UploadSnapshot()
	}
	return nil, nil
}

func (this DataManagerProtectedEntityTypeManager) CopyFromInfo(ctx context.Context, info astrolabe.ProtectedEntityInfo, params map[string]map[string]interface{}, options astrolabe.CopyCreateOptions) (astrolabe.ProtectedEntity, error) {
	fmt.Println("CopyFromInfo called")
	return nil, nil
}
