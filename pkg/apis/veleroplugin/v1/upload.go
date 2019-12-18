package v1

import (
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// UploadSpec is the specification for Upload resource
type UploadSpec struct {
	// SnapshotID is the identifier for the snapshot of the volume.
	SnapshotID string `json:"snapshotID,omitempty"`

	// BackupTimestamp records the time the backup was called.
	// The server's time is used for SnapshotTimestamp
	BackupTimestamp *meta_v1.Time `json:"backupTimestamp,omitempty"`
}

// UploadPhase represents the lifecycle phase of a Upload.
// +kubebuilder:validation:Enum=New;InProgress;Completed;Failed
type UploadPhase string

const (
	UploadPhaseNew        UploadPhase = "New"
	UploadPhaseInProgress UploadPhase = "InProgress"
	UploadPhaseCompleted  UploadPhase = "Completed"
	UploadPhaseFailed     UploadPhase = "Failed"
)

// UploadStatus is the current status of a Upload.
type UploadStatus struct {
	// Phase is the current state of the Upload.
	// +optional
	Phase UploadPhase `json:"phase,omitempty"`

	// Message is a message about the upload's status.
	// +optional
	Message string `json:"message,omitempty"`

	// StartTimestamp records the time an upload was started.
	// The server's time is used for StartTimestamps
	// +optional
	// +nullable
	StartTimestamp *meta_v1.Time `json:"startTimestamp,omitempty"`

	// CompletionTimestamp records the time an upload was completed.
	// Completion time is recorded even on failed uploads.
	// The server's time is used for CompletionTimestamps
	// +optional
	// +nullable
	CompletionTimestamp *meta_v1.Time `json:"completionTimestamp,omitempty"`

	// Progress holds the total number of bytes of the volume and the current
	// number of backed up bytes. This can be used to display progress information
	// about the backup operation.
	// +optional
	Progress UploadOperationProgress `json:"progress,omitempty"`

	// The DataManager node that has picked up the Upload for processing.
	// This will be updated as soon as the Upload is picked up for processing.
	// If the DataManager couldn't process Upload for some reason it will be picked up by another
	// node.
	ProcessingNode string `json:"processingNode,omitempty"`
}

// UploadOperationProgress represents the progress of a
// Upload operation
type UploadOperationProgress struct {
	// +optional
	TotalBytes int64 `json:"totalBytes,omitempty"`

	// +optional
	BytesDone int64 `json:"bytesDone,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Upload describe a velero-plugin backup
type Upload struct {
	// TypeMeta is the metadata for the resource, like kind and apiversion
	meta_v1.TypeMeta `json:",inline"`

	// +optional
	meta_v1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the custom resource spec
	Spec UploadSpec `json:"spec"`

	// +optional
	Status UploadStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// UploadList is a list of Upload resources
type UploadList struct {
	meta_v1.TypeMeta `json:",inline"`

	// +optional
	meta_v1.ListMeta `json:"metadata,omitempty"`

	Items []Upload `json:"items"`
}
