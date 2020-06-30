package backupdriver

import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/pkg/errors"
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
	backupdriverV1Client *v1.BackupdriverV1Client) (*backupdriverv1.BackupRepository, error) {
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
	var backupRepository *backupdriverv1.BackupRepository
	var backupRepositoryClaim *backupdriverv1.BackupRepositoryClaim
	brcList, err := backupdriverV1Client.BackupRepositoryClaims(ns).List(metav1.ListOptions{})
	if err != nil {
		return nil, errors.Errorf("Failed to list backup repository claims from the %v namespace", ns)
	}
	if len(brcList.Items) > 0 {
		// Pre-existing BackupRepositoryClaims found.
		// Assuming only the first one, in theory multiple backup repo claim could exist.
		repositoryClaimFound := brcList.Items[0]
		if repositoryClaimFound.BackupRepository != "" {
			// The BackupRepositoryClaim is not associated with any BackupRepository
			// TODO: Search and assign or create a new BR and assign.
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
	return backupRepository, nil
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
		// Compare the params. RepositoryDriver is always same.
		equal := reflect.DeepEqual(backupRepositoryClaim.RepositoryParameters, backupRepository.RepositoryParameters)
		if !equal {
			return
		}
		equal = reflect.DeepEqual(backupRepositoryClaim.RepositoryDriver, backupRepository.RepositoryDriver)
		if !equal {
			return
		}
		// TODO: verify allowed namespaces.
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

