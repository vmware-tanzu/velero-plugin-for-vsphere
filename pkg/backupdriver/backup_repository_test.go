package backupdriver

import (
	"context"
	"fmt"
	backupdriverv1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/backupdriver/v1"
	v1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned/typed/backupdriver/v1"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
	"log"
	"testing"
	"time"
)

func TestClaimBackupRepository(t *testing.T) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	// if you want to change the loading rules (which files in which order), you can do so here

	configOverrides := &clientcmd.ConfigOverrides{}
	// if you want to change override values or bind them to flags, there are methods to help you

	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	config, err := kubeConfig.ClientConfig()
	if err != nil {
		log.Panic(err.Error())
	}
	clientset, err := v1.NewForConfig(config)
	if err != nil {
		log.Panic(err.Error())
	}
	testBackupRepository := backupdriverv1.BackupRepository{
		TypeMeta:              metav1.TypeMeta{
			Kind:       "BackupRepository",
			APIVersion: "backupdriver.io/v1",
		},
		ObjectMeta:            metav1.ObjectMeta{
			Name:                       "backup-repo-1",
		},
		AllowedNamespaces:     []string{},
		RepositoryDriver:      utils.S3RepositoryDriver,
		RepositoryParameters:  map[string]string{},
		BackupRepositoryClaim: "",
	}
	// Delete any duplicate BRs
	err = clientset.BackupRepositories().Delete(testBackupRepository.Name, nil)
	if err != nil {
		fmt.Printf("BackupRepository Delete error = %v\n", err)
	}
	newBackupRepository, err := clientset.BackupRepositories().Create(&testBackupRepository)
	if err != nil {
		fmt.Printf("BackupRepository Create error = %v\n", err)
	}
	fmt.Printf("BackupRepository Created successfully = %v\n", newBackupRepository)
	timeoutContext, _ := context.WithDeadline(context.Background(), time.Now().Add(time.Second * 10))
	backupRepo, err := ClaimBackupRepository(timeoutContext,
		utils.S3RepositoryDriver,
		map[string]string{},[]string{},
		"test", clientset)
	if err != nil {
		fmt.Printf("ClaimBackupRepository returned err = %v\n", err)
	} else {
		fmt.Printf("ClaimBackupRepository returned BackupRepository = %s\n", backupRepo)
	}
}
