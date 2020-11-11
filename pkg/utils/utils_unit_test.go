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
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/constants"
	"k8s.io/client-go/rest"
	"strings"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	backupdriverapi "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/backupdriver/v1alpha1"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned/fake"
	backupdriverTypedV1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned/typed/backupdriver/v1alpha1"
	informers "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/informers/externalversions"
	veleroplugintest "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/test"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func TestDeleteSvcSnapshot(t *testing.T) {
	tests := []struct {
		name                     string
		gcSnapshot               *backupdriverapi.Snapshot
		svcSnapshot              *backupdriverapi.Snapshot
		config                   *rest.Config
		toDeleteSvcSnapshotFirst bool
		expectedErr              bool
	}{
		{
			name: "If svcSnapshot has already been deleted, should not return error",
			gcSnapshot: &backupdriverapi.Snapshot{
				TypeMeta: metav1.TypeMeta{
					APIVersion: backupdriverapi.SchemeGroupVersion.String(),
					Kind:       "Snapshot",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Name:      "gc-snapshot-1",
				},
				Status: backupdriverapi.SnapshotStatus{
					SvcSnapshotName: "svc-snapshot-1",
				},
			},
			svcSnapshot: &backupdriverapi.Snapshot{
				TypeMeta: metav1.TypeMeta{
					APIVersion: backupdriverapi.SchemeGroupVersion.String(),
					Kind:       "Snapshot",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Name:      "svc-snapshot-1",
				},
			},
			config:                   &rest.Config{},
			toDeleteSvcSnapshotFirst: true,
			expectedErr:              false,
		},
		{
			name: "Delete a corresponding svc snapshot from gc snapshot",
			gcSnapshot: &backupdriverapi.Snapshot{
				TypeMeta: metav1.TypeMeta{
					APIVersion: backupdriverapi.SchemeGroupVersion.String(),
					Kind:       "Snapshot",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Name:      "gc-snapshot-2",
				},
				Status: backupdriverapi.SnapshotStatus{
					SvcSnapshotName: "svc-snapshot-2",
				},
			},
			svcSnapshot: &backupdriverapi.Snapshot{
				TypeMeta: metav1.TypeMeta{
					APIVersion: backupdriverapi.SchemeGroupVersion.String(),
					Kind:       "Snapshot",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Name:      "svc-snapshot-2",
				},
			},
			config:                   &rest.Config{},
			toDeleteSvcSnapshotFirst: false,
			expectedErr:              false,
		},
		{
			name: "Guest cluster with no svccnapshot name should return error",
			gcSnapshot: &backupdriverapi.Snapshot{
				TypeMeta: metav1.TypeMeta{
					APIVersion: backupdriverapi.SchemeGroupVersion.String(),
					Kind:       "Snapshot",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Name:      "gc-snapshot-3",
				},
			},
			svcSnapshot: &backupdriverapi.Snapshot{
				TypeMeta: metav1.TypeMeta{
					APIVersion: backupdriverapi.SchemeGroupVersion.String(),
					Kind:       "Snapshot",
				},
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "test-ns",
					Name:      "svc-snapshot-3",
				},
			},
			config:                   &rest.Config{},
			toDeleteSvcSnapshotFirst: false,
			expectedErr:              true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var (
				client             = fake.NewSimpleClientset(test.svcSnapshot)
				sharedInformers    = informers.NewSharedInformerFactory(client, 0)
				logger             = veleroplugintest.NewLogger()
				backupdriverClient = client.BackupdriverV1alpha1()
			)
			require.NoError(t, sharedInformers.Backupdriver().V1alpha1().Snapshots().Informer().GetStore().Add(test.gcSnapshot))
			if !test.toDeleteSvcSnapshotFirst {
				require.NoError(t, sharedInformers.Backupdriver().V1alpha1().Snapshots().Informer().GetStore().Add(test.svcSnapshot))
			}

			patches := gomonkey.ApplyFunc(GetBackupdriverClient, func(_ *rest.Config) (backupdriverTypedV1.BackupdriverV1alpha1Interface, error) {
				return backupdriverClient, nil
			})
			defer patches.Reset()
			patches.ApplyFunc(GetSupervisorConfig, func(_ *rest.Config, _ logrus.FieldLogger) (*rest.Config, string, error) {
				return &rest.Config{}, test.svcSnapshot.Namespace, nil
			})
			result := DeleteSvcSnapshot(test.gcSnapshot.Status.SvcSnapshotName, test.gcSnapshot.Namespace, test.gcSnapshot.Name, test.config, logger)
			if test.expectedErr {
				require.NotNil(t, result)
			} else {
				require.Nil(t, result)
			}
		})
	}
}

func TestGetComponentFromImage(t *testing.T) {
	tests := []struct {
		name                                             string
		image                                            string
		expectedRepo, expectedContainer, expectedVersion string
	}{
		{
			name:              "ExpectedDummyCase",
			image:             "a/b/c/d:x",
			expectedRepo:      "a/b/c",
			expectedContainer: "d",
			expectedVersion:   "x",
		},
		{
			name:              "ExpectedDockerhubCase",
			image:             "vsphereveleroplugin/velero-plugin-for-vsphere:1.1.0-rc2",
			expectedRepo:      "vsphereveleroplugin",
			expectedContainer: "velero-plugin-for-vsphere",
			expectedVersion:   "1.1.0-rc2",
		},
		{
			name:              "ExpectedCustomizedCase",
			image:             "xyz-repo.vmware.com/velero/velero-plugin-for-vsphere:1.1.0-rc2",
			expectedRepo:      "xyz-repo.vmware.com/velero",
			expectedContainer: "velero-plugin-for-vsphere",
			expectedVersion:   "1.1.0-rc2",
		},
		{
			name:              "ExpectedLocalCase",
			image:             "velero-plugin-for-vsphere:1.1.0-rc2",
			expectedRepo:      "",
			expectedContainer: "velero-plugin-for-vsphere",
			expectedVersion:   "1.1.0-rc2",
		},
		{
			name:              "ExpectedNonTaggedImageCase",
			image:             "xyz-repo.vmware.com/velero/velero-plugin-for-vsphere",
			expectedRepo:      "xyz-repo.vmware.com/velero",
			expectedContainer: "velero-plugin-for-vsphere",
			expectedVersion:   "",
		},
		{
			name:              "ExpectedCaseOfRegistryEndpointWithPort",
			image:             "xyz-repo.vmware.com:9999/velero/velero-plugin-for-vsphere:1.1.0-rc2",
			expectedRepo:      "xyz-repo.vmware.com:9999/velero",
			expectedContainer: "velero-plugin-for-vsphere",
			expectedVersion:   "1.1.0-rc2",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actualRepo := GetComponentFromImage(test.image, constants.ImageRepositoryComponent)
			actualContainer := GetComponentFromImage(test.image, constants.ImageContainerComponent)
			actualVersion := GetComponentFromImage(test.image, constants.ImageVersionComponent)
			assert.Equal(t, actualRepo, test.expectedRepo)
			assert.Equal(t, actualContainer, test.expectedContainer)
			assert.Equal(t, actualVersion, test.expectedVersion)
		})
	}
}
