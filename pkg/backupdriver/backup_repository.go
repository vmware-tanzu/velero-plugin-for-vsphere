package backupdriver


import (
	"context"
	"fmt"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	backupdriverv1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/backupdriver/v1"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/builder"
	v1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned/typed/backupdriver/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/tools/cache"
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
			if apierrors.IsNotFound(err) {
				// BR was deleted before access.
				// TODO: Create a new BR and cross reference the BR and BRC?.
			}
			if err != nil {
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
		backupRepoClaimName := backupRepoClaimUUID.String()
		backupRepositoryClaimReq := builder.ForBackupRepositoryClaim(ns, backupRepoClaimName).
			RepositoryParameters(repositoryParameters).RepositoryDriver().
			AllowedNamespaces(allowedNamespaces).Result()
		_, err = backupdriverV1Client.BackupRepositoryClaims(ns).Create(backupRepositoryClaimReq)
		if err != nil {
			return nil, errors.Errorf("Failed to create backup repository claim with name %v in namespace %v", backupRepoClaimName, ns)
		}
		// Wait here till a BR is assigned or created for the BRC.
		results := make(chan waitBRResult)
		watchlist := cache.NewListWatchFromClient(backupdriverV1Client.RESTClient(),
			"backuprepository", metav1.NamespacePublic, fields.Everything())
		_, controller := cache.NewInformer(
			watchlist,
			&backupdriverv1.BackupRepository{},
			time.Second*0,
			cache.ResourceEventHandlerFuncs{
				AddFunc: func(obj interface{}) {
					backupRepo := obj.(*backupdriverv1.BackupRepository)
					fmt.Printf("Backup Repository added: %v", backupRepo)
					checkBackupRepositoryIsClaimed(backupRepoClaimName, backupRepo, results)
				},
				UpdateFunc: func(oldObj, newObj interface{}) {
					backupRepoNew := newObj.(*backupdriverv1.BackupRepository)
					fmt.Printf("Backup Repository updated to: %v", backupRepoNew)
					checkBackupRepositoryIsClaimed(backupRepoClaimName, backupRepoNew, results)
				},
				DeleteFunc: func(obj interface{}) {
					backupRepo := obj.(*backupdriverv1.BackupRepository)
					fmt.Printf("Backup Repository deleted: %v", backupRepo)
					// Nothing to do.
				},
			})
		stop := make(chan struct{})
		go controller.Run(stop)
		select {
		case <-ctx.Done():
			stop <- struct{}{}
			return nil, ctx.Err()
		case result := <-results:
			return result.backupRepository, nil
		}
	}
	return backupRepository, nil
}

// checkBackupRepositoryIsClaimed checks if the the BackupRepository is associated with
// a specific claim, if so it writes the BackupRepository into the result channel
func checkBackupRepositoryIsClaimed(backupRepositoryClaimName string,
	backupRepository *backupdriverv1.BackupRepository,
	result chan waitBRResult) {
	if backupRepository.BackupRepositoryClaim == backupRepositoryClaimName {
		result <- waitBRResult{
			backupRepository: backupRepository,
			err:              nil,
		}
	}
}
