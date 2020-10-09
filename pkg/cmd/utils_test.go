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

package cmd

import (
	"github.com/stretchr/testify/assert"
	"testing"
	v1 "k8s.io/api/core/v1"
)

func TestGetVersionFromImage(t *testing.T) {
	tests := []struct {
		name       string
		key        string
		containers []v1.Container
		expected   string
	} {
		{
			name:       "Valid image string should return non-empty version",
			key:        "cloud-provider-vsphere/csi/release/driver",
			containers: []v1.Container{
				{
					Image: "gcr.io/cloud-provider-vsphere/csi/release/driver:v1.0.1",
				},
			},
			expected:   "v1.0.1",
		},
		{
			name:       "Valid image string should return non-empty version",
			key:        "cloud-provider-vsphere/csi/release/driver",
			containers: []v1.Container{
				{
					Image: "cloud-provider-vsphere/csi/release/driver:v2.0.0",
				},
			},
			expected:   "v2.0.0",
		},
		{
			name:       "Valid image string should return non-empty version",
			key:        "cloud-provider-vsphere/csi/release/driver",
			containers: []v1.Container{
				{
					Image: "myregistry/cloud-provider-vsphere/csi/release/driver:v2.0.0",
				},
			},
			expected:   "v2.0.0",
		},
		{
			name:       "Valid image string should return non-empty version",
			key:        "cloud-provider-vsphere/csi/release/driver",
			containers: []v1.Container{
				{
					Image: "myregistry/level1/level2/cloud-provider-vsphere/csi/release/driver:v2.0.0",
				},
			},
			expected:   "v2.0.0",
		},
		{
			name:     "Invalid image name should return empty string",
			key:      "cloud-provider-vsphere/csi/release/driver",
			containers: []v1.Container{
				{
					Image: "gcr.io/csi/release/driver:v1.0.1",
				},
			},
			expected: "",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			version := GetVersionFromImage(test.containers, test.key)
			assert.Equal(t, test.expected, version)
		})
	}
}
