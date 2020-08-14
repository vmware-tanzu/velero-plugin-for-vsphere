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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	veleroplugintest "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/test"
	"testing"
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

func TestGetRepo(t *testing.T) {
	tests := []struct {
		name     string
		image    string
		expected string
	} {
		{
			name:     "Top level registry",
			image:    "harbor.mylab.local/velero-plugin-for-vsphere:1.0.1",
			expected: "harbor.mylab.local",
		},
		{
			name :    "Multiple level registry",
			image:    "harbor.mylab.local/library/velero-plugin-for-vsphere:1.0.1",
			expected: "harbor.mylab.local/library",
		},
		{
			name :    "No / should return empty string",
			image:    "velero-plugin-for-vsphere:1.0.1",
			expected: "",
		},
		{
			name :    "/ appears in beginning should return empty string",
			image:    "/velero-plugin-for-vsphere:1.0.1",
			expected: "",
		},
		{
			name :    "Empty input should return empty string",
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
