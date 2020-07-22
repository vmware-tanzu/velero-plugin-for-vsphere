package paravirt

import (
	"context"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/vmware-tanzu/astrolabe/pkg/astrolabe"
	"github.com/vmware-tanzu/astrolabe/pkg/pvc"
	"github.com/vmware-tanzu/astrolabe/pkg/util"
	backupdriverTypedV1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned/typed/backupdriver/v1"
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

type ParaVirtProtectedEntityTypeManager struct {
	entityType            ParaVirtEntityType
	gcKubeClientSet       *kubernetes.Clientset
	svcKubeClientSet      *kubernetes.Clientset
	svcBackupDriverClient *backupdriverTypedV1.BackupdriverV1Client // we might want to change to BackupdriverV1Interface later
	svcNamespace          string
	s3Config              astrolabe.S3Config
	logger                logrus.FieldLogger
}

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

	// Get the supervisor namespace where the paravirtualized PETM is running at
	svcNamespace := params["svcNamespace"].(string)

	// Fill in ParaVirt PETM
	return &ParaVirtProtectedEntityTypeManager{
		entityType:            entityType,
		gcKubeClientSet:       gcKubeClientSet,
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

	pvList, err := this.gcKubeClientSet.CoreV1().PersistentVolumes().List(metav1.ListOptions{})
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

func (this *ParaVirtProtectedEntityTypeManager) Copy(ctx context.Context, pe astrolabe.ProtectedEntity, options astrolabe.CopyCreateOptions) (astrolabe.ProtectedEntity, error) {
	panic("implement me")
}

func (this *ParaVirtProtectedEntityTypeManager) CopyFromInfo(ctx context.Context, info astrolabe.ProtectedEntityInfo, options astrolabe.CopyCreateOptions) (astrolabe.ProtectedEntity, error) {
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
