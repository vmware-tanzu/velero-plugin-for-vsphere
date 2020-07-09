package backupdriver

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	backupdriverv1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/backupdriver/v1"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/builder"
	v1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned/typed/backupdriver/v1"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/utils"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/tools/cache"
	"reflect"
	"time"
)

type waitBRResult struct {
	backupRepository *backupdriverv1.BackupRepository
	err              error
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
	backupdriverV1Client *v1.BackupdriverV1Client,
	logger logrus.FieldLogger) (*backupdriverv1.BackupRepository, error) {
	/*
		1. Look for BRC in the "ns" specified.
		2. If there are no BRC in the namespace create one.
		3. If there are pre-existing BRCs in the namespace, for each BRC
			a) check if the backup repository name is populated, if populated
				nothing more to do.
			b) If not, iterate through existing BR that are not associated with any
				BRCs and match the BRC (repo parameters and namespaces), update the
				BR to reference the BRC.
		4. If there are no pre-existing BRC, create one. Wait for it to be associated with
			a BR
	*/
	var backupRepositoryClaim *backupdriverv1.BackupRepositoryClaim
	brcList, err := backupdriverV1Client.BackupRepositoryClaims(ns).List(metav1.ListOptions{})
	if err != nil {
		return nil, errors.Errorf("Failed to list backup repository claims from the %v namespace", ns)
	}
	if len(brcList.Items) > 0 {
		logger.Infof("Found %d BackupRepositoryClaims", len(brcList.Items))
		// Pre-existing BackupRepositoryClaims found.
		// Assuming only the first one, in theory multiple backup repo claim could exist.
		repositoryClaimFound := brcList.Items[0]
		if repositoryClaimFound.BackupRepository != "" {
			// The BackupRepositoryClaim is not associated with any BackupRepository
			// Search and assign or create a new BR and assign.
			foundBackupRepo, err := SearchAndClaimUnclaimedBackupRepository(ctx, repositoryDriver, repositoryParameters,
				allowedNamespaces, repositoryClaimFound.Name, backupdriverV1Client, logger)
			if err != nil {
				return nil, err
			}
			if foundBackupRepo != nil {
				logger.Infof("Found and claimed a pre-existing BackupRepository successfully")
				// Patch the BRC with the BR name.
				err = patchBackupRepositoryClaim(&repositoryClaimFound,
					foundBackupRepo.Name, ns, backupdriverV1Client)
				if err != nil {
					logger.Errorf("Failed to patch the BRC with BR reference")
					return nil, err
				}
				return foundBackupRepo, nil
			}
			// The BRC found no matching BR. Explicitly create one.
			brNew, err := CreateBackupRepository(ctx, repositoryDriver, repositoryParameters, allowedNamespaces,
				repositoryClaimFound.Name, backupdriverV1Client, logger)
			if err != nil {
				logger.Errorf("Failed to create BackupRepository")
			}
			if brNew == nil {
				return nil, errors.Errorf("Unexpected, the BackupRepository was not created and no errors were found")
			}
			// Patch the BRC with the BR name.
			err = patchBackupRepositoryClaim(&repositoryClaimFound,
				brNew.Name, ns, backupdriverV1Client)
			if err != nil {
				logger.Errorf("Failed to patch the BRC with BR reference")
				return nil, err
			}
			// Return the BackupRepository
			return brNew, nil
		} else {
			// Retrieve the BackupRepository and return.
			backupRepositoryNameFound := repositoryClaimFound.BackupRepository
			backupRepositoryFound, err := backupdriverV1Client.BackupRepositories().
				Get(backupRepositoryNameFound, metav1.GetOptions{})
			if err != nil {
				if apierrors.IsNotFound(err) {
					// BR was deleted before access.
					// TODO: Trigger a new BR creation and cross reference the BR and BRC?.
				}
				return nil, errors.Errorf("Failed to retrieve backup repository %v", err)
			}
			return backupRepositoryFound, nil
		}
	} else {
		// No existing BackupRepositoryClaim in the specified guest cluster namespace.
		// Create a new BackupRepositoryClaim.
		backupRepoClaimUUID, err := uuid.NewRandom()
		if err != nil {
			return nil, errors.Errorf("Failed to generate backup repository claim name")
		}
		backupRepoClaimName := "brc-" + backupRepoClaimUUID.String()
		backupRepositoryClaimReq := builder.ForBackupRepositoryClaim(ns, backupRepoClaimName).
			RepositoryParameters(repositoryParameters).RepositoryDriver().
			AllowedNamespaces(allowedNamespaces).Result()
		backupRepositoryClaim, err = backupdriverV1Client.BackupRepositoryClaims(ns).Create(backupRepositoryClaimReq)
		if err != nil {
			return nil, errors.Errorf("Failed to create backup repository claim with name %v in namespace %v", backupRepoClaimName, ns)
		}
		// Wait here till a BR is assigned or created for the BRC.
		results := make(chan waitBRResult)
		watchlist := cache.NewListWatchFromClient(backupdriverV1Client.RESTClient(),
			"backuprepositories", metav1.NamespaceNone, fields.Everything())
		_, controller := cache.NewInformer(
			watchlist,
			&backupdriverv1.BackupRepository{},
			time.Second*0,
			cache.ResourceEventHandlerFuncs{
				AddFunc: func(obj interface{}) {
					backupRepo := obj.(*backupdriverv1.BackupRepository)
					fmt.Printf("Backup Repository added: %v", backupRepo)
					checkIfBackupRepositoryIsClaimed(backupRepositoryClaim, backupRepo, results)
				},
				UpdateFunc: func(oldObj, newObj interface{}) {
					backupRepoNew := newObj.(*backupdriverv1.BackupRepository)
					fmt.Printf("Backup Repository updated to: %v", backupRepoNew)
					checkIfBackupRepositoryIsClaimed(backupRepositoryClaim, backupRepoNew, results)
				},
			})
		stop := make(chan struct{})
		go controller.Run(stop)
		// TODO: Remove anon function once the backup driver is able to create BR.
		go func() {
			_, err = CreateBackupRepository(ctx, repositoryDriver, repositoryParameters, allowedNamespaces,
				backupRepoClaimName, backupdriverV1Client, logger)
			if err != nil {
				logger.Errorf("Failed to create BackupRepository")
			}
		}()
		select {
		case <-ctx.Done():
			stop <- struct{}{}
			return nil, ctx.Err()
		case result := <-results:
			err := patchBackupRepositoryClaim(backupRepositoryClaim,
				result.backupRepository.Name, ns, backupdriverV1Client)
			if err != nil {
				return nil, err
			}
			return result.backupRepository, nil
		}
	}
}

// checkIfBackupRepositoryIsClaimed checks if the the BackupRepository is associated with
// a specific claim, if so it writes the BackupRepository into the result channel
func checkIfBackupRepositoryIsClaimed(
	backupRepositoryClaim *backupdriverv1.BackupRepositoryClaim,
	backupRepository *backupdriverv1.BackupRepository,
	result chan waitBRResult) {
	if backupRepository.BackupRepositoryClaim == backupRepositoryClaim.Name {
		// If the BR was created explicitly with the claim name return it.
		result <- waitBRResult{
			backupRepository: backupRepository,
			err:              nil,
		}
	} else if backupRepository.BackupRepositoryClaim == "" {
		// Unclaimed BackupRepository.
		match := compareBackupRepository(backupRepositoryClaim.RepositoryDriver, backupRepositoryClaim.RepositoryParameters, backupRepositoryClaim.AllowedNamespaces, backupRepository)
		if !match {
			return
		}
		result <- waitBRResult{
			backupRepository: backupRepository,
			err:              nil,
		}
	}
}

// Patch the BackupRepositoryClaim with the BackupRepository name.
func patchBackupRepositoryClaim(backupRepositoryClaim *backupdriverv1.BackupRepositoryClaim,
	backRepositoryName string,
	ns string,
	backupdriverV1Client *v1.BackupdriverV1Client) error {
	mutate := func(r *backupdriverv1.BackupRepositoryClaim) {
		backupRepositoryClaim.BackupRepository = backRepositoryName
	}
	_, err := utils.PatchBackupRepositoryClaim(backupRepositoryClaim, mutate, backupdriverV1Client.BackupRepositoryClaims(ns))
	if err != nil {
		return errors.Errorf("Failed to patch backup repository claim %v with backup repository %v in namespace %v", backupRepositoryClaim.Name, backRepositoryName, ns)
	}
	return nil
}

// Patch the BackupRepository with the BackupRepositoryClaim name.
func patchBackupRepository(backupRepository *backupdriverv1.BackupRepository,
	backRepositoryClaimName string,
	backupdriverV1Client *v1.BackupdriverV1Client) error {
	mutate := func(r *backupdriverv1.BackupRepository) {
		backupRepository.BackupRepositoryClaim = backRepositoryClaimName
	}
	_, err := utils.PatchBackupRepository(backupRepository, mutate, backupdriverV1Client.BackupRepositories())
	if err != nil {
		return errors.Errorf("Failed to patch backup repository %v with backup repository claim %v", backupRepository, backRepositoryClaimName)
	}
	return nil
}

// Creates a BackupRepository with the parameters.
func CreateBackupRepository(ctx context.Context,
	repositoryDriver string,
	repositoryParameters map[string]string,
	allowedNamespaces []string,
	backupRepositoryClaimName string,
	backupdriverV1Client *v1.BackupdriverV1Client,
	logger logrus.FieldLogger) (*backupdriverv1.BackupRepository, error) {

	logger.Infof("Creating BackupRepository for the BackupRepositoryClaim %s", backupRepositoryClaimName)
	backupRepoUUID, err := uuid.NewRandom()
	if err != nil {
		return nil, errors.Errorf("Failed to generate backup repository name")
	}
	backupRepoName := "br-" + backupRepoUUID.String()
	backupRepoReq := builder.ForBackupRepository(backupRepoName).
		BackupRepositoryClaim(backupRepositoryClaimName).
		AllowedNamespaces(allowedNamespaces).
		RepositoryParameters(repositoryParameters).
		RepositoryDriver().Result()
	br, err := backupdriverV1Client.BackupRepositories().Create(backupRepoReq)
	if err != nil {
		logger.Errorf("Failed to created the BackupRepository CRD")
		return nil, err
	}
	logger.Infof("Successfully created BackupRepository %s for the BackupRepositoryClaim %s", backupRepoName, backupRepositoryClaimName)
	return br, nil
}

// Search for unclaimed BackupRepository with the parameters.
// TODO: Handle race conditions with creates.
func SearchAndClaimUnclaimedBackupRepository(ctx context.Context,
	repositoryDriver string,
	repositoryParameters map[string]string,
	allowedNamespaces []string,
	backupRepositoryClaimName string,
	backupdriverV1Client *v1.BackupdriverV1Client,
	logger logrus.FieldLogger) (*backupdriverv1.BackupRepository, error) {
	backupRepositoryList, err := backupdriverV1Client.BackupRepositories().List(metav1.ListOptions{})
	if err != nil {
		logger.Errorf("Failed to retrieve existing BackupRepositories")
		return nil, err
	}
	if len(backupRepositoryList.Items) > 0 {
		for _, backupRepository := range backupRepositoryList.Items {
			if backupRepository.BackupRepositoryClaim == "" {
				// Consider only unclaimed BackupRepository
				match := compareBackupRepository(repositoryDriver, repositoryParameters, allowedNamespaces, &backupRepository)
				if match {
					logger.Infof("Found unclaimed matching BackupRepository %s for BackupRepositoryClaim %s", backupRepository.Name, backupRepositoryClaimName)
					// Patch the BR.
					err := patchBackupRepository(&backupRepository, backupRepositoryClaimName, backupdriverV1Client)
					if err != nil {
						return nil, err
					}
					return &backupRepository, nil
				}
			}
		}
	} else {
		logger.Infof("No pre existing BackupRepositories found.")
	}
	return nil, nil
}

func compareBackupRepository(repositoryDriver string,
	repositoryParameters map[string]string,
	allowedNamespaces []string,
	backupRepository *backupdriverv1.BackupRepository) bool {
	// Compare the params. RepositoryDriver is always same.
	equal := reflect.DeepEqual(repositoryParameters, backupRepository.RepositoryParameters)
	if !equal {
		return false
	}
	equal = reflect.DeepEqual(repositoryDriver, backupRepository.RepositoryDriver)
	if !equal {
		return false
	}
	equal = reflect.DeepEqual(allowedNamespaces, backupRepository.AllowedNamespaces)
	if !equal {
		return false
	}
	return true
}
