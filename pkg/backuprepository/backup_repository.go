package backuprepository

import (
	"context"
	"encoding/json"
	"fmt"
	jsonpatch "github.com/evanphx/json-patch"
	"github.com/vmware-tanzu/astrolabe/pkg/s3repository"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/constants"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/utils"
	"k8s.io/apimachinery/pkg/types"
	"reflect"
	"time"

	"k8s.io/client-go/rest"

	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	backupdriverv1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/backupdriver/v1alpha1"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/builder"
	backupdriverTypedV1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned/typed/backupdriver/v1alpha1"
	v1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned/typed/backupdriver/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/tools/cache"
)

type waitBRCResult struct {
	backupRepositoryName string
	err                  error
}

/*
This will either create a new BackupRepositoryClaim record or update/use an existing BackupRepositoryClaim record. In
either case, it does not return until the BackupRepository is assigned and ready to use or the context is canceled
(in which case it will return an error).
*/
func ClaimBackupRepository(ctx context.Context,
	repositoryDriver string,
	repositoryParameters map[string]string,
	allowedNamespaces []string,
	ns string,
	backupdriverV1Client *v1.BackupdriverV1alpha1Client,
	logger logrus.FieldLogger) (string, error) {

	// The map holds all the BRCs that match the parameters.
	// Any BR that references any of the BRC in the map can be returned.
	brcMap := make(map[string]*backupdriverv1.BackupRepositoryClaim)

	brcList, err := backupdriverV1Client.BackupRepositoryClaims(ns).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return "", errors.Errorf("Failed to list backup repository claims from the %v namespace", ns)
	}
	logger.Infof("Found %d BackupRepositoryClaims", len(brcList.Items))
	// Pre-existing BackupRepositoryClaims found.
	for _, repositoryClaimItem := range brcList.Items {
		// Process the BRC only if its they match all the params.
		repoMatch := compareBackupRepositoryClaim(repositoryDriver, repositoryParameters, allowedNamespaces, &repositoryClaimItem, logger)
		if repoMatch {
			if repositoryClaimItem.BackupRepository == "" {
				logger.Infof("Found matching BRC for the parameters with no BR reference, BRC: %s", repositoryClaimItem.Name)
				logger.Infof("Continuing processing other BRC, the backupdriver is expected to process this BRC later")
				brcMap[repositoryClaimItem.Name] = &repositoryClaimItem
			} else {
				logger.Infof("Found matching BRC for the parameters, BRC: %s", repositoryClaimItem.Name)
				// Retrieve the BackupRepository and return.
				backupRepositoryNameFound := repositoryClaimItem.BackupRepository
				return backupRepositoryNameFound, nil
			}
		}
	}

	if len(brcMap) == 0 {
		// No existing BackupRepositoryClaim in the specified guest cluster namespace,
		// Or no matching BackupRepositoryClaim among them.
		// Create a new BackupRepositoryClaim to indicate the params.
		backupRepoClaimUUID, err := uuid.NewRandom()
		if err != nil {
			return "", errors.Errorf("Failed to generate backup repository claim name")
		}
		backupRepoClaimName := "brc-" + backupRepoClaimUUID.String()
		backupRepositoryClaimReq := builder.ForBackupRepositoryClaim(ns, backupRepoClaimName).
			RepositoryParameters(repositoryParameters).RepositoryDriver().
			AllowedNamespaces(allowedNamespaces).Result()
		backupRepositoryClaim, err := backupdriverV1Client.BackupRepositoryClaims(ns).Create(context.TODO(), backupRepositoryClaimReq, metav1.CreateOptions{})
		if err != nil {
			return "", errors.Errorf("Failed to create backup repository claim with name %v in namespace %v", backupRepoClaimName, ns)
		}
		brcMap[backupRepoClaimName] = backupRepositoryClaim
	}
	results := make(chan waitBRCResult)
	watchlist := cache.NewListWatchFromClient(backupdriverV1Client.RESTClient(),
		"backuprepositoryclaims", ns, fields.Everything())
	_, controller := cache.NewInformer(
		watchlist,
		&backupdriverv1.BackupRepositoryClaim{},
		time.Second*0,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				backupRepoClaim := obj.(*backupdriverv1.BackupRepositoryClaim)
				logger.Debugf("Backup Repository Claim added: %s", backupRepoClaim.Name)
				checkIfBackupRepositoryClaimIsReferenced(brcMap, backupRepoClaim, results, logger)
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				backupRepoClaimNew := newObj.(*backupdriverv1.BackupRepositoryClaim)
				logger.Debugf("Backup Repository Claim: %s updated", backupRepoClaimNew.Name)
				checkIfBackupRepositoryClaimIsReferenced(brcMap, backupRepoClaimNew, results, logger)
			},
		})
	stop := make(chan struct{})
	go controller.Run(stop)

	// Wait here till a BR is assigned or created for the BRC.
	select {
	case <-ctx.Done():
		stop <- struct{}{}
		return "", ctx.Err()
	case result := <-results:
		return result.backupRepositoryName, nil
	}
}

// Creates a BackupRepository with the parameters.
func CreateBackupRepository(ctx context.Context,
	brc *backupdriverv1.BackupRepositoryClaim,
	svcBrName string,
	backupdriverV1Client *v1.BackupdriverV1alpha1Client,
	logger logrus.FieldLogger) (*backupdriverv1.BackupRepository, error) {

	logger.Infof("Creating BackupRepository for the BackupRepositoryClaim %s", brc.Name)
	backupRepoName := GetBackupRepositoryNameForBackupRepositoryClaim(brc)
	var backupRepoReq *backupdriverv1.BackupRepository
	// Check if the BackupRepository already exists.
	backupRepoReq, err := backupdriverV1Client.BackupRepositories().
		Get(context.TODO(), backupRepoName, metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			logger.Errorf("Error occurred trying to retrieve BackupRepository API object %s: %v",
				backupRepoName, err)
			return nil, errors.Errorf("failed to retrieve BackupRepository API object %s: %v", backupRepoName, err)
		}
		backupRepoReq = nil
	}

	// BackupRepository not found. Create a new one
	if backupRepoReq == nil {
		backupRepoReq = builder.ForBackupRepository(backupRepoName).
			BackupRepositoryClaim(brc.Name).
			AllowedNamespaces(brc.AllowedNamespaces).
			RepositoryParameters(brc.RepositoryParameters).
			RepositoryDriver().
			SvcBackupRepositoryName(svcBrName).Result()
		newBackupRepo, err := backupdriverV1Client.BackupRepositories().Create(context.TODO(), backupRepoReq, metav1.CreateOptions{})
		if err != nil {
			logger.Errorf("Failed to create the BackupRepository API object: %v", err)
			return nil, err
		}
		logger.Infof("Successfully created BackupRepository API object %s for the BackupRepositoryClaim %s",
			backupRepoName, brc.Name)

		return newBackupRepo, nil
	}
	logger.Infof("Found BackupRepository %s for the BackupRepositoryClaim %s", backupRepoReq.Name, brc.Name)

	return backupRepoReq, nil
}

// Construct a unique name for BackupRepository based on UID of BackupRepositoryClaim
func GetBackupRepositoryNameForBackupRepositoryClaim(brc *backupdriverv1.BackupRepositoryClaim) string {
	return "br-" + string(brc.UID)
}

/*
 * Only called from Guest Cluster. This will either create a new
 * BackupRepositoryClaim record or update/use an existing BackupRepositoryClaim
 * record in the Supervisor namespace. In either case, it does not return until
 * the BackupRepository is assigned in the supervisor cluster.
 */
func ClaimSvcBackupRepository(ctx context.Context,
	brc *backupdriverv1.BackupRepositoryClaim,
	svcConfig *rest.Config,
	svcNamespace string,
	logger logrus.FieldLogger) (string, error) {

	svcBackupdriverClient, err := backupdriverTypedV1.NewForConfig(svcConfig)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return ClaimBackupRepository(ctx, brc.RepositoryDriver, brc.RepositoryParameters, []string{svcNamespace}, svcNamespace, svcBackupdriverClient, logger)
}

func checkIfBackupRepositoryClaimIsReferenced(
	brcMap map[string]*backupdriverv1.BackupRepositoryClaim,
	backupRepositoryClaim *backupdriverv1.BackupRepositoryClaim,
	result chan waitBRCResult,
	logger logrus.FieldLogger) {
	brcName := backupRepositoryClaim.Name
	brRefName := backupRepositoryClaim.BackupRepository
	if brRefName != "" {
		// Check if the BRC is among any of the ones we are interested in.
		if _, ok := brcMap[brcName]; ok {
			logger.Infof("Found matching BackupRepositoryClaim: %s with referenced BackupRepository: %s",
				brcName, brRefName)
			result <- waitBRCResult{
				backupRepositoryName: brRefName,
				err:                  nil,
			}
		}
	} else {
		logger.Infof("Received BackupRepositoryClaim with no BackupRepository reference, ignoring it now, as it"+
			"is expected to be patched later, BackupRepositoryClaim: %v", backupRepositoryClaim)
	}
}

// Patch the BackupRepositoryClaim with the BackupRepository name.
func PatchBackupRepositoryClaim(backupRepositoryClaim *backupdriverv1.BackupRepositoryClaim,
	backRepositoryName string,
	ns string,
	backupdriverV1Client *v1.BackupdriverV1alpha1Client) error {
	mutate := func(r *backupdriverv1.BackupRepositoryClaim) {
		backupRepositoryClaim.BackupRepository = backRepositoryName
	}
	_, err := patchBackupRepositoryClaimInt(backupRepositoryClaim, mutate, backupdriverV1Client.BackupRepositoryClaims(ns))
	if err != nil {
		return errors.Errorf("Failed to patch backup repository claim %v with backup repository %v in namespace %v", backupRepositoryClaim.Name, backRepositoryName, ns)
	}
	return nil
}

func patchBackupRepositoryClaimInt(req *backupdriverv1.BackupRepositoryClaim,
	mutate func(*backupdriverv1.BackupRepositoryClaim),
	backupRepoClaimClient v1.BackupRepositoryClaimInterface) (*backupdriverv1.BackupRepositoryClaim, error) {

	// Record original json
	oldData, err := json.Marshal(req)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to marshall original BackupRepositoryClaim")
	}

	// Mutate
	mutate(req)

	// Record new json
	newData, err := json.Marshal(req)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to marshall updated BackupRepositoryClaim")
	}

	patchBytes, err := jsonpatch.CreateMergePatch(oldData, newData)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to creat json merge patch for BackupRepositoryClaim")
	}

	req, err = backupRepoClaimClient.Patch(context.TODO(), req.Name, types.MergePatchType, patchBytes, metav1.PatchOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to patch BackupRepositoryClaim")
	}
	return req, nil
}

func compareBackupRepositoryClaim(repositoryDriver string,
	repositoryParameters map[string]string,
	allowedNamespaces []string,
	backupRepositoryClaim *backupdriverv1.BackupRepositoryClaim,
	logger logrus.FieldLogger) bool {
	// Compare the params. RepositoryDriver is always same.
	logger.Infof("Comparing BRC: %v", backupRepositoryClaim)
	equal := reflect.DeepEqual(repositoryParameters, backupRepositoryClaim.RepositoryParameters)
	if !equal {
		logger.Infof("repositoryParameters not matched")
		return false
	}
	equal = repositoryDriver == backupRepositoryClaim.RepositoryDriver
	if !equal {
		logger.Infof("repositoryDriver not matched")
		return false
	}
	equal = reflect.DeepEqual(allowedNamespaces, backupRepositoryClaim.AllowedNamespaces)
	if !equal {
		return false
	}
	return true
}

func GetRepositoryFromBackupRepository(backupRepository *backupdriverv1.BackupRepository, logger logrus.FieldLogger) (*s3repository.ProtectedEntityTypeManager, error) {
	switch backupRepository.RepositoryDriver {
	case constants.S3RepositoryDriver:
		params := make(map[string]interface{})
		for k, v := range backupRepository.RepositoryParameters {
			params[k] = v
		}
		return utils.GetS3PETMFromParamsMap(params, logger)
	default:
		errMsg := fmt.Sprintf("Unsupported backuprepository driver type: %s. Only support %s.", backupRepository.RepositoryDriver, constants.S3RepositoryDriver)
		return nil, errors.New(errMsg)
	}
}

func GetBackupRepositoryFromBackupRepositoryName(backupRepositoryName string) (*backupdriverv1.BackupRepository, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, errors.Wrap(err, "Failed to get k8s inClusterConfig")
	}
	pluginClient, err := versioned.NewForConfig(config)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to get k8s clientset from the given config: %v ", config)
		return nil, errors.Wrapf(err, errMsg)
	}
	backupRepositoryCR, err := pluginClient.BackupdriverV1alpha1().BackupRepositories().Get(context.TODO(), backupRepositoryName, metav1.GetOptions{})
	if err != nil {
		errMsg := fmt.Sprintf("Error while retrieving the backup repository CR %v", backupRepositoryName)
		return nil, errors.Wrapf(err, errMsg)
	}
	return backupRepositoryCR, nil
}

func RetrieveBackupRepositoryFromBSL(ctx context.Context, bslName string, pvcNamespace string, veleroNs string,
	backupdriverClient *v1.BackupdriverV1alpha1Client, restConfig *rest.Config, logger logrus.FieldLogger) (string, error) {
	var backupRepositoryName string
	logger.Info("Claiming backup repository")

	repositoryParameters := make(map[string]string)
	err := utils.RetrieveParamsFromBSL(repositoryParameters, bslName, restConfig, logger)
	if err != nil {
		logger.Errorf("Failed to translate BSL to repository parameters: %v", err)
		return backupRepositoryName, errors.WithStack(err)
	}
	backupRepositoryName, err = ClaimBackupRepository(ctx, constants.S3RepositoryDriver, repositoryParameters,
		[]string{pvcNamespace}, veleroNs, backupdriverClient, logger)
	if err != nil {
		logger.Errorf("Failed to claim backup repository: %v", err)
		return backupRepositoryName, errors.WithStack(err)
	}
	return backupRepositoryName, nil
}
