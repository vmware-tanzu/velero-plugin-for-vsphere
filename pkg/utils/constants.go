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

import "time"

const (
	// supported volume type in plugin
	CnsBlockVolumeType = "ivd"
)

const (
	// Duration at which lease expires on CRs.
	LeaseDuration = 60 * time.Second

	// Duration after which leader renews its lease.
	RenewDeadline = 15 * time.Second

	// Duration after which non-leader retries to acquire lease.
	RetryPeriod = 5 * time.Second
)

const (
	// Duration after which Reflector resyncs CRs and calls UpdateFunc on each of the existing CRs.
	ResyncPeriod = 30 * time.Second
)

// configuration constants for the volume snapshot plugin
const (
	// The key of SnapshotManager mode for data movement. Specifically, boolean string values are expected.
	// By default, it is "false". No data movement from local to remote storage if "true" is set.
	VolumeSnapshotterLocalMode = "LocalMode"
	// The key of SnapshotManager location
	VolumeSnapshotterManagerLocation = "SnapshotManagerLocation"
	// Valid values for the config with the VolumeSnapshotterManagerLocation key
	VolumeSnapshotterPlugin     = "Plugin"
	VolumeSnapshotterDataServer = "DataServer"
)

const (
	// Max retry limit for downloads.
	DOWNLOAD_MAX_RETRY = 5

	// Initial retry for both uploads and downloads.
	MIN_RETRY = 0

	// BACKOFF for downloads.
	DOWNLOAD_BACKOFF = 5

	// Max backoff limit for uploads.
	UPLOAD_MAX_BACKOFF = 60

	// Exceeds this number of retry, will give a warning message to ask user to fix network issue in cluster.
	RETRY_WARNING_COUNT = 8
)

// configuration constants for the S3 repository
const (
	DefaultS3RepoPrefix = "plugins/vsphere-astrolabe-repo"
)
const (
	// Minimum velero version number to meet velero plugin requirement
	VeleroMinVersion = "v1.3.2"

	// Minimum csi driver version number to meet velero plugin requirement
	CsiMinVersion = "v1.0.2"
)

const (
	// DefaultNamespace is the Kubernetes namespace that is used by default for
	// the Velero server and API objects.
	DefaultNamespace = "velero"
)

const (
	// Default port used to access vCenter.
	DefaultVCenterPort string = "443"
)

const (
	DataManagerForPlugin string = "data-manager-for-plugin"

	VeleroPluginForVsphere string = "velero-plugin-for-vsphere"

	VeleroDeployment string = "velero"
)

const (
	S3RepositoryDriver string = "s3repository.astrolabe.vmware-tanzu.com"
)

const (
	// VSphereItemActionPluginFlag is the feature flag string that defines whether or not vSphere ItemActionPlugin features are being used.
	VSphereItemActionPluginFlag = "EnableVSphereItemActionPlugin"
)
