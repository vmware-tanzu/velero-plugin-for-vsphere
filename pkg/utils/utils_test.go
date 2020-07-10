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
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	veleroplugintest "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/test"
	v1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/backupdriver/v1"
	"testing"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetStringFromParamsMap(t *testing.T) {
    params := make(map[string]interface{})
    params["ValidKey"] = "ValidValue"
    params["NonString"] = false
    tests := []struct{
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
	tests := []struct{
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
			name:     "Empty string",
			str:      "",
			defValue: false,
                        expectedVal: false,
		},
		{
			name:     "Invalid str",
			str:      "AAA",
			defValue: true,
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
	tests := []struct{
		name             string
		key              string
		backupRepository *v1.BackupRepository
		expectedErr      error
	}{
		{
			name:             "Unsupported backup driver type returns error",
			key:              "backupdriver/backuprepository-1",
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
			expectedErr:      errors.New("Unsupported backuprepository driver type: unsupported-driver. Only support s3repository.astrolabe.vmware-tanzu.com."),
		},
		{
			name: "Repository parameter missing region should return error",
			key:   "miss-region",
			backupRepository: &v1.BackupRepository{
			    TypeMeta: metav1.TypeMeta{
			        APIVersion: v1.SchemeGroupVersion.String(),
			        Kind:       "BackupRepository",
		        },
			    ObjectMeta: metav1.ObjectMeta{
			        Name: "default",
		        },
			    RepositoryDriver: S3RepositoryDriver,
			    RepositoryParameters: map1,
		    },
			expectedErr: errors.New("Missing region param, cannot initialize S3 PETM"),
		},
		{
			name: "Repository parameter missing bucket should return error",
			key:   "miss-bucket",
			backupRepository: &v1.BackupRepository{
				TypeMeta: metav1.TypeMeta{
					APIVersion: v1.SchemeGroupVersion.String(),
					Kind:       "BackupRepository",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
				RepositoryDriver: S3RepositoryDriver,
				RepositoryParameters: map2,
			},
			expectedErr: errors.New("Missing bucket param, cannot initialize S3 PETM"),
		},
	}
	for _, test := range tests {
		var (
			logger          = veleroplugintest.NewLogger()
		)

		t.Run(test.name, func(t *testing.T) {
			_, err := GetRepositoryFromBackupRepository(test.backupRepository, logger)
			assert.Equal(t, test.expectedErr.Error(), err.Error())
		})
	}
}