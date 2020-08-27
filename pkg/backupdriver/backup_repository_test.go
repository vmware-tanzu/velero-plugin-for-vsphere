package backupdriver

import (
	"context"
	"github.com/vmware-tanzu/velero/pkg/generated/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"os"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	backupdriverv1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/backupdriver/v1"
	backupdriverTypedV1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned/typed/backupdriver/v1"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/utils"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
)

func TestClaimBackupRepository(t *testing.T) {
	path := os.Getenv("KUBECONFIG")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("The KubeConfig file, %v, is not exist", path)
	}

	config, err := clientcmd.BuildConfigFromFlags("", path)
	if err != nil {
		t.Fatalf("Failed to build k8s config from kubeconfig file: %+v ", err)
	}

	// Setup Logger
	logger := logrus.New()
	formatter := new(logrus.TextFormatter)
	formatter.TimestampFormat = time.RFC3339Nano
	formatter.FullTimestamp = true
	logger.SetFormatter(formatter)
	logger.SetLevel(logrus.DebugLevel)

	// using velero ns for testing.
	veleroNs := "velero"

	backupdriverClient, err := backupdriverTypedV1.NewForConfig(config)
	if err != nil {
		t.Fatalf("Failed to retrieve backupdriverClient from config: %v", config)
	}
	repositoryParameters := make(map[string]string)
	ctx := context.Background()
	testDoneStatus := make(chan bool)

	// The following anon function triggers ClaimBackupRepository and waits for the BR
	go func() {
		backupRepositoryName, err := ClaimBackupRepository(ctx, utils.S3RepositoryDriver, repositoryParameters,
			[]string{"test"}, veleroNs, backupdriverClient, logger)
		if err != nil {
			t.Fatalf("Failed to retrieve the BackupRepository name.")
		}
		logger.Infof("Successfully retrieved the BackupRepository name: %s", backupRepositoryName)
		// Trigger a test complete.
		testDoneStatus <- true
	}()

	// Wait for the BRC to be created and create corresponding BR.
	watchlist := cache.NewListWatchFromClient(backupdriverClient.RESTClient(),
		"backuprepositoryclaims", veleroNs, fields.Everything())
	_, controller := cache.NewInformer(
		watchlist,
		&backupdriverv1.BackupRepositoryClaim{},
		time.Second*0,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				backupRepoClaim := obj.(*backupdriverv1.BackupRepositoryClaim)
				logger.Infof("Backup Repository Claim added: %s", backupRepoClaim.Name)
				err = handleNewBackupRepositoryClaim(ctx, backupRepoClaim, veleroNs, backupdriverClient, logger)
				if err != nil {
					t.Fatalf("The test failed while processing new BRC.")
				}
				logger.Infof("Successfully created BR and patched BRC to point to BR.")
			},
		})
	stop := make(chan struct{})
	go controller.Run(stop)
	select {
	case <-ctx.Done():
		stop <- struct{}{}
	case <-testDoneStatus:
		logger.Infof("Test completed successfully")
	}
}

func TestBackupRepositoryCreationFromBSL(t *testing.T) {
	path := os.Getenv("KUBECONFIG")
	ctx := context.Background()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("The KubeConfig file, %v, is not exist", path)
	}

	config, err := clientcmd.BuildConfigFromFlags("", path)
	if err != nil {
		t.Fatalf("Failed to build k8s config from kubeconfig file: %+v ", err)
	}

	// Setup Logger
	logger := logrus.New()
	formatter := new(logrus.TextFormatter)
	formatter.TimestampFormat = time.RFC3339Nano
	formatter.FullTimestamp = true
	logger.SetFormatter(formatter)
	logger.SetLevel(logrus.DebugLevel)

	// using velero ns for testing.
	veleroNs := "velero"

	veleroClient, err := versioned.NewForConfig(config)
	if err != nil {
		t.Fatalf("Failed to retrieve veleroClient")
	}

	backupdriverClient, err := backupdriverTypedV1.NewForConfig(config)
	if err != nil {
		t.Fatalf("Failed to retrieve backupdriverClient from config: %v", config)
	}

	backupStorageLocationList, err := veleroClient.VeleroV1().BackupStorageLocations(veleroNs).List(context.TODO(), metav1.ListOptions{})
	if err != nil || len(backupStorageLocationList.Items) <= 0 {
		t.Fatalf("RetrieveVSLFromVeleroBSLs: Failed to list Velero default backup storage location")
	}
	for _, item := range backupStorageLocationList.Items {
		repositoryParameters := make(map[string]string)
		bslName := item.Name
		err := utils.RetrieveParamsFromBSL(repositoryParameters, bslName, config, logger)
		if err != nil {
			logger.Errorf("RetrieveParamsFromBSL Failed %v", err)
			t.Fatalf("RetrieveParamsFromBSL failed!")
		}
		logger.Infof("Repository Parameters: %v", repositoryParameters)
		backupRepositoryName, err := ClaimBackupRepository(ctx, utils.S3RepositoryDriver, repositoryParameters,
			[]string{"test"}, veleroNs, backupdriverClient, logger)
		if err != nil {
			t.Fatalf("Failed to retrieve the BackupRepository name.")
		}
		logger.Infof("Successfully retrieved the BackupRepository name: %s", backupRepositoryName)
		// TODO: Manually verify in supervisor cluster for the corresponding BR.
	}
}

// This function creates a new BR in response to the BRC.
// Patches the BRC with the BR
func handleNewBackupRepositoryClaim(ctx context.Context,
	backupRepositoryClaim *backupdriverv1.BackupRepositoryClaim,
	ns string,
	backupdriverClient *backupdriverTypedV1.BackupdriverV1Client,
	logger logrus.FieldLogger) error {
	backupRepository, err := CreateBackupRepository(ctx, backupRepositoryClaim, "", backupdriverClient, logger)
	if err != nil {
		logger.Errorf("Failed to create the BackupRepository")
		return err
	}
	err = PatchBackupRepositoryClaim(backupRepositoryClaim, backupRepository.Name, ns, backupdriverClient)
	if err != nil {
		logger.Errorf("Failed to patch the BRC with the newly created BR")
	}
	return nil
}
