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
	"os"
	"strings"
	"testing"
	"time"

	k8sv1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/backupdriver/v1"
	backupdriverTypedV1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned/typed/backupdriver/v1"
	veleroplugintest "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/test"
	"github.com/vmware-tanzu/velero/pkg/generated/clientset/versioned"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/clientcmd"
)

func TestGetStringFromParamsMap(t *testing.T) {
	params := make(map[string]interface{})
	params["ValidKey"] = "ValidValue"
	params["NonString"] = false
	tests := []struct {
		name          string
		key           string
		expectedValue string
		ok            bool
	}{
		{
			name:          "Valid key with string value should return corresponding value and true",
			key:           "ValidKey",
			expectedValue: "ValidValue",
			ok:            true,
		},
		{
			name:          "If value in map is non-string should return empty string and false",
			key:           "NonString",
			expectedValue: "",
			ok:            false,
		},
		{
			name:          "No such key should return empty string and false",
			key:           "NoSuchKey",
			expectedValue: "",
			ok:            false,
		},
	}

	logger := veleroplugintest.NewLogger()

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			str, ok := GetStringFromParamsMap(params, test.key, logger)
			assert.Equal(t, test.expectedValue, str)
			assert.Equal(t, test.ok, ok)
		})
	}
}

func TestGetBool(t *testing.T) {
	tests := []struct {
		name        string
		str         string
		defValue    bool
		expectedVal bool
	}{
		{
			name:        "Pos1",
			str:         "true",
			defValue:    false,
			expectedVal: true,
		},
		{
			name:        "Pos2",
			str:         "false",
			defValue:    true,
			expectedVal: false,
		},
		{
			name:        "Empty string",
			str:         "",
			defValue:    false,
			expectedVal: false,
		},
		{
			name:        "Invalid str",
			str:         "AAA",
			defValue:    true,
			expectedVal: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			res := GetBool(test.str, test.defValue)
			require.Equal(t, test.expectedVal, res)
		})
	}
}

func TestCreateRepositoryFromBackupRepository(t *testing.T) {
	map1 := make(map[string]string)
	map2 := make(map[string]string)
	map2["region"] = "us-west-1"
	tests := []struct {
		name             string
		key              string
		backupRepository *v1.BackupRepository
		expectedErr      error
	}{
		{
			name: "Unsupported backup driver type returns error",
			key:  "backupdriver/backuprepository-1",
			backupRepository: &v1.BackupRepository{
				TypeMeta: metav1.TypeMeta{
					APIVersion: v1.SchemeGroupVersion.String(),
					Kind:       "BackupRepository",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
				RepositoryDriver: "unsupported-driver",
			},
			expectedErr: errors.New("Unsupported backuprepository driver type: unsupported-driver. Only support s3repository.astrolabe.vmware-tanzu.com."),
		},
		{
			name: "Repository parameter missing region should return error",
			key:  "miss-region",
			backupRepository: &v1.BackupRepository{
				TypeMeta: metav1.TypeMeta{
					APIVersion: v1.SchemeGroupVersion.String(),
					Kind:       "BackupRepository",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
				RepositoryDriver:     S3RepositoryDriver,
				RepositoryParameters: map1,
			},
			expectedErr: errors.New("Missing region param, cannot initialize S3 PETM"),
		},
		{
			name: "Repository parameter missing bucket should return error",
			key:  "miss-bucket",
			backupRepository: &v1.BackupRepository{
				TypeMeta: metav1.TypeMeta{
					APIVersion: v1.SchemeGroupVersion.String(),
					Kind:       "BackupRepository",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
				RepositoryDriver:     S3RepositoryDriver,
				RepositoryParameters: map2,
			},
			expectedErr: errors.New("Missing bucket param, cannot initialize S3 PETM"),
		},
	}
	for _, test := range tests {
		var (
			logger = veleroplugintest.NewLogger()
		)

		t.Run(test.name, func(t *testing.T) {
			_, err := GetRepositoryFromBackupRepository(test.backupRepository, logger)
			assert.Equal(t, test.expectedErr.Error(), err.Error())
		})
	}
}

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

func TestRerieveVcConfigSecret(t *testing.T) {
	// Setup Logger
	logger := logrus.New()
	formatter := new(logrus.TextFormatter)
	formatter.TimestampFormat = time.RFC3339Nano
	formatter.FullTimestamp = true
	logger.SetFormatter(formatter)
	logger.SetLevel(logrus.DebugLevel)

	tests := []struct {
		name     string
		sEnc     string
		vc       string
		password string
	}{
		{
			name:     "Password with special character \\ in it",
			sEnc:     "[VirtualCenter \"sc-rdops-vm06-dhcp-184-231.eng.vmware.com\"]\npassword = \"GpI4G`OK'?in40Fo/0\\\\;\"",
			vc:       "sc-rdops-vm06-dhcp-184-231.eng.vmware.com",
			password: "GpI4G`OK'?in40Fo/0\\;",
		},
		{
			name:     "Password with multiple = in it",
			sEnc:     "[VirtualCenter \"sc-rdops-vm06-dhcp-184-231.eng.vmware.com\"]\npassword = \"GpI4G`OK'?in40Fo/0\\\\;=h=\"",
			vc:       "sc-rdops-vm06-dhcp-184-231.eng.vmware.com",
			password: "GpI4G`OK'?in40Fo/0\\;=h=",
		},
		{
			name:     "Password with special character \\t in it",
			sEnc:     "[VirtualCenter \"sc-rdops-vm06-dhcp-184-231.eng.vmware.com\"]\npassword = \"G4\\t4t\"",
			vc:       "sc-rdops-vm06-dhcp-184-231.eng.vmware.com",
			password: "G4\t4t",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			lines := strings.Split(test.sEnc, "\n")
			params := make(map[string]interface{})
			ParseLines(lines, params, logger)
			assert.Equal(t, test.vc, params["VirtualCenter"])
			assert.Equal(t, test.password, params["password"])
		})
	}
}

func TestGetRepo(t *testing.T) {
	tests := []struct {
		name     string
		image    string
		expected string
	}{
		{
			name:     "Top level registry",
			image:    "harbor.mylab.local/velero-plugin-for-vsphere:1.0.1",
			expected: "harbor.mylab.local",
		},
		{
			name:     "Multiple level registry",
			image:    "harbor.mylab.local/library/velero-plugin-for-vsphere:1.0.1",
			expected: "harbor.mylab.local/library",
		},
		{
			name:     "No / should return empty string",
			image:    "velero-plugin-for-vsphere:1.0.1",
			expected: "",
		},
		{
			name:     "/ appears in beginning should return empty string",
			image:    "/velero-plugin-for-vsphere:1.0.1",
			expected: "",
		},
		{
			name:     "Empty input should return empty string",
			image:    "",
			expected: "",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := GetRepo(test.image)
			assert.Equal(t, test.expected, repo)
		})
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
			Name: PvSecretName,
		},
	}

	// set up logger
	logger := logrus.New()
	formatter := new(logrus.TextFormatter)
	formatter.TimestampFormat = time.RFC3339
	formatter.FullTimestamp = true
	logger.SetFormatter(formatter)

	err = clientSet.CoreV1().Secrets(BackupDriverNamespace).Delete(context.TODO(), testSecret.Name, metav1.DeleteOptions{})
	if err != nil {
		t.Fatalf("Delete error = %v\n", err)
	}
	writtenSnapshot, err := clientSet.CoreV1().Secrets(BackupDriverNamespace).Create(context.TODO(), &testSecret, metav1.CreateOptions{})

	testSecret.ObjectMeta = writtenSnapshot.ObjectMeta

	timeoutContext, cancelFunc := context.WithDeadline(context.Background(), time.Now().Add(time.Second*10))
	defer cancelFunc()
	createdSecret, err := waitForPvSecret(timeoutContext, clientSet, BackupDriverNamespace, logger)
	if err != nil {
		t.Fatalf("waitForPvSecret returned err = %v\n", err)
	} else {
		fmt.Printf("waitForPvSecret secret created %s in namespace %s\n", createdSecret.Name, createdSecret.Namespace)
	}

}
