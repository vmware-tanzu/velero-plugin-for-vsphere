package v1

import (
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DownloadSpec is the specification for Download resource
type DownloadSpec struct {
	// Node is the name of the node that the Pod is running on.
	Node string `json:"node"`

	// SnapshotID is the identifier for the snapshot of the pod volume.
	SnapshotID string `json:"snapshotID,omitempty"`

	// Volume is the name of the volume within the Pod to be restored.
	Volume string `json:"volume"`
}

// DownloadPhase represents the lifecycle phase of a Download.
// +kubebuilder:validation:Enum=New;InProgress;Completed;Failed
type DownloadPhase string

const (
	DownloadPhaseNew        DownloadPhase = "New"
	DownloadPhaseInProgress DownloadPhase = "InProgress"
	DownloadPhaseCompleted  DownloadPhase = "Completed"
	DownloadPhaseFailed     DownloadPhase = "Failed"
)

// DownloadStatus is the current status of a Download.
type DownloadStatus struct {
	// Phase is the current state of the Download.
	// +optional
	Phase DownloadPhase `json:"phase,omitempty"`

	// Message is a message about the download's status.
	// +optional
	Message string `json:"message,omitempty"`

	// StartTimestamp records the time a backup was started.
	// Separate from CreationTimestamp, since that value changes
	// on restores.
	// The server's time is used for StartTimestamps
	// +optional
	// +nullable
	StartTimestamp *meta_v1.Time `json:"startTimestamp,omitempty"`

	// CompletionTimestamp records the time a backup was completed.
	// Completion time is recorded even on failed backups.
	// Completion time is recorded before uploading the backup object.
	// The server's time is used for CompletionTimestamps
	// +optional
	// +nullable
	CompletionTimestamp *meta_v1.Time `json:"completionTimestamp,omitempty"`

	// Progress holds the total number of bytes of the volume and the current
	// number of restore up bytes. This can be used to display progress information
	// about the restore operation.
	// +optional
	Progress DownloadOperationProgress `json:"progress,omitempty"`
}

// DownloadOperationProgress represents the progress of a
// Download operation
type DownloadOperationProgress struct {
	// +optional
	TotalBytes int64 `json:"totalBytes,omitempty"`

	// +optional
	BytesDone int64 `json:"bytesDone,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Download describe a velero-plugin restore
type Download struct {
	// TypeMeta is the metadata for the resource, like kind and apiversion
	meta_v1.TypeMeta `json:",inline"`

	// +optional
	meta_v1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the custom resource spec
	Spec DownloadSpec `json:"spec"`

	// +optional
	Status DownloadStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// UploadList is a list of Upload resources
type DownloadList struct {
	meta_v1.TypeMeta `json:",inline"`

	// +optional
	meta_v1.ListMeta `json:"metadata,omitempty"`

	Items []Upload `json:"items"`
}
