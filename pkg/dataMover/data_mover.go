package dataMover

import (
	"context"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/sirupsen/logrus"
	"github.com/vmware-tanzu/astrolabe/pkg/astrolabe"
	"github.com/vmware-tanzu/astrolabe/pkg/ivd"
	"github.com/vmware-tanzu/astrolabe/pkg/s3repository"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/utils"
	"net/url"
	"os"
)

type DataMover struct {
	logrus.FieldLogger
	ivdPETM *ivd.IVDProtectedEntityTypeManager
	s3PETM  *s3repository.ProtectedEntityTypeManager
}

func NewDataMoverFromCluster(logger logrus.FieldLogger) (*DataMover, error) {
	params := make(map[string]interface{})
	var err error
	debugMode := os.Getenv("DEBUGMODE") != ""
	logger.Infof("Data mover is running in the debug mode: %v", debugMode)
	if debugMode {
		err = utils.RetrieveVcConfigSecretByHardCoding(params, logger)
	} else {
		err = utils.RetrieveVcConfigSecret(params, logger)
	}

	if err != nil {
		logger.Errorf("Could not retrieve vsphere credential from k8s secret with error message: %v", err)
		return nil, err
	}

	err = utils.RetrieveBackupStorageLocation(params, logger)
	if err != nil {
		logger.Errorf("Could not retrieve velero default backup location with error message: %v", err)
		return nil, err
	}

	dataMover, err := newDataMoverFromParamsMap(params, logger)
	if err != nil {
		logger.Errorf("Failed to new a snapshot manager from params map")
		return nil, err
	}
	return dataMover, nil
}

func newDataMoverFromParamsMap(params map[string]interface{}, logger logrus.FieldLogger) (*DataMover, error) {
	var vcUrl url.URL
	vcUrl.Scheme = "https"
	vcHostStr, ok := params["VirtualCenter"].(string)
	if !ok {
		return nil, errors.New("Missing vcHost param, cannot initialize SnapshotManager")
	}
	vcHostPortStr, ok := params["port"].(string)
	if !ok {
		return nil, errors.New("Missing port param, cannot initialize SnapshotManager")
	}

	vcUrl.Host = fmt.Sprintf("%s:%s", vcHostStr, vcHostPortStr)

	vcUser, ok := params["user"].(string)
	if !ok {
		return nil, errors.New("Missing vcUser param, cannot initialize SnapshotManager")
	}
	vcPassword, ok := params["password"].(string)
	if !ok {
		return nil, errors.New("Missing vcPassword param, cannot initialize SnapshotManager")
	}
	vcUrl.User = url.UserPassword(vcUser, vcPassword)
	vcUrl.Path = "/sdk"

	insecure := false
	insecureStr, ok := params["insecure-flag"].(string)
	if ok && (insecureStr == "TRUE" || insecureStr == "true") {
		insecure = true
	}

	region := params["region"].(string)
	bucket := params["bucket"].(string)

	s3URLBase := fmt.Sprintf("https://s3-%s.amazonaws.com/%s/", region, bucket)
	serviceType := "ivd"

	// init ivd PETM
	ivdPETM, err := ivd.NewIVDProtectedEntityTypeManagerFromURL(&vcUrl, s3URLBase, insecure, logger)
	if err != nil {
		logger.Errorf("Error at calling ivd API, NewIVDProtectedEntityTypeManagerFromURL")
		return nil, err
	}

	sess := session.Must(session.NewSession(&aws.Config{
		Region: aws.String(region),
	}))

	s3PETM, err := s3repository.NewS3RepositoryProtectedEntityTypeManager(serviceType, *sess, bucket, logger)
	if err != nil {
		logger.Errorf("Error at creating new ProtectedEntityTypeManager for s3 repository")
		return nil, err
	}

	dataMover := DataMover{
		FieldLogger: logger,
		ivdPETM: ivdPETM,
		s3PETM:  s3PETM,
	}

	logger.Infof("Data Moving Manager is initialized")
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