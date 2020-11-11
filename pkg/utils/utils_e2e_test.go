/*
Copyright 2020 the Velero contributors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package utils

import (
	"context"
	"fmt"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/constants"
	"os"
	"testing"
	"time"

	k8sv1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	backupdriverTypedV1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned/typed/backupdriver/v1alpha1"
	"github.com/vmware-tanzu/velero/pkg/generated/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
)

func TestRetrieveParamsFromBSL(t *testing.T) {
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

	veleroClient, err := versioned.NewForConfig(config)
	if err != nil {
		t.Fatalf("Failed to retrieve veleroClient")
	}

	_, err = backupdriverTypedV1.NewForConfig(config)
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
		err := RetrieveParamsFromBSL(repositoryParameters, bslName, config, logger)
		if err != nil {
			logger.Errorf("Retrieve Failed %v", err)
			t.Fatalf("RetrieveParamsFromBSL failed!")
		}
		logger.Infof("Repository Parameters: %v", repositoryParameters)
	}
}




func createClientSet() (*kubernetes.Clientset, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	// if you want to change the loading rules (which files in which order), you can do so here

	configOverrides := &clientcmd.ConfigOverrides{}
	// if you want to change override values or bind them to flags, there are methods to help you

	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	config, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, NewClientConfigNotFoundError("Could not create client config")
	}

	clientset, err := kubernetes.NewForConfig(config)

	if err != nil {
		return nil, errors.Wrap(err, "Could not create clientset")
	}
	return clientset, err
}

type ClientConfigNotFoundError struct {
	errMsg string
}

func (this ClientConfigNotFoundError) Error() string {
	return this.errMsg
}

func NewClientConfigNotFoundError(errMsg string) ClientConfigNotFoundError {
	err := ClientConfigNotFoundError{
		errMsg: errMsg,
	}
	return err
}

/*
 * For this test, set KUBECONFIG to point to a valid setup, with a BackupDriverNamespace created.
 */
func Test_waitForPvSecret(t *testing.T) {
	clientSet, err := createClientSet()

	if err != nil {
		_, ok := err.(ClientConfigNotFoundError)
		if ok {
			t.Skip(err)
		}
		t.Fatal(err)
	}
	testSecret := k8sv1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: constants.PvSecretName,
		},
	}

	// set up logger
	logger := logrus.New()
	formatter := new(logrus.TextFormatter)
	formatter.TimestampFormat = time.RFC3339
	formatter.FullTimestamp = true
	logger.SetFormatter(formatter)

	err = clientSet.CoreV1().Secrets(constants.BackupDriverNamespace).Delete(context.TODO(), testSecret.Name, metav1.DeleteOptions{})
	if err != nil {
		t.Fatalf("Delete error = %v\n", err)
	}
	writtenSnapshot, err := clientSet.CoreV1().Secrets(constants.BackupDriverNamespace).Create(context.TODO(), &testSecret, metav1.CreateOptions{})

	testSecret.ObjectMeta = writtenSnapshot.ObjectMeta

	timeoutContext, cancelFunc := context.WithDeadline(context.Background(), time.Now().Add(time.Second*10))
	defer cancelFunc()
	createdSecret, err := waitForPvSecret(timeoutContext, clientSet, constants.BackupDriverNamespace, logger)
	if err != nil {
		t.Fatalf("waitForPvSecret returned err = %v\n", err)
	} else {
		fmt.Printf("waitForPvSecret secret created %s in namespace %s\n", createdSecret.Name, createdSecret.Namespace)
	}

}
