package v1

import (
	core_v1 "k8s.io/api/core/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type SnapshotSpec struct {
	// ResourceHandle refers to a Kubernetes resource, currently a PVC but this may be extended in the future
	core_v1.TypedLocalObjectReference `json:"resourceHandle"`

	// The backup repository to snapshot into.  The namespace the Snapshot/PVC lives in must have access to the repository
	BackupRepository string `json:"backupRepository"`

	// SnapshotCancel indicates request to cancel ongoing snapshot.  SnapshotCancel can be set at anytime before
	// the snapshot reaches a terminal phase.  If the snapshot has reached a terminal phase
	SnapshotCancel bool `json:"snapshotCancel,omitempty"`
}

// SnapshotPhase represents the lifecycle phase of a Snapshot.
// New - No work yet, next phase is InProgress
// InProgress - snapshot being taken
// Snapshotted - local snapshot complete, next phase is Protecting or SnapshotFailed
// SnapshotFailed - end state, snapshot was not able to be taken
// Uploading - snapshot is being moved to durable storage
// Uploaded - end state, snapshot has been protected
// UploadFailed - end state, unable to move to durable storage
// Canceling - when the SanpshotCancel flag is set, if the Snapshot has not already moved into a terminal state, the
//             status will move to Canceling.  The snapshot ID will be removed from the status status if has been filled in
//             and the snapshot ID will not longer be valid for a Clone operation
// Canceled - the operation was canceled, the snapshot ID is not valid
type SnapshotPhase string

const (
	SnapshotPhaseNew            SnapshotPhase = "New"
	SnapshotPhaseInProgress     SnapshotPhase = "InProgress"
	SnapshotPhaseSnapshotted    SnapshotPhase = "Snapshotted"
	SnapshotPhaseSnapshotFailed SnapshotPhase = "SnapshotFailed"
	SnapshotPhaseUploading      SnapshotPhase = "Uploading"
	SnapshotPhaseUploaded       SnapshotPhase = "Uploaded"
	SnapshotPhaseUploadFailed   SnapshotPhase = "UploadFailed"
	SnapshotPhaseCanceling      SnapshotPhase = "Canceling"
	SnapshotPhaseCanceled       SnapshotPhase = "Canceled"
)

// UploadOperationProgress represents the progress of a
// Upload operation
type SnapshotProgress struct {
	// +optional
	TotalBytes int64 `json:"totalBytes,omitempty"`

	// +optional
	BytesDone int64 `json:"bytesDone,omitempty"`
}

type SnapshotStatus struct {
	// Phase is the current state of the Upload.
	Phase SnapshotPhase `json:"phase,omitempty"`

	// Message is a message about the snapshot's status.
	// +optional
	Message string `json:"message,omitempty"`

	// Progress for the upload
	Progress SnapshotProgress `json:"progress,omitempty"`

	// Snapshot ID that has been taken.  This will be filled in when the phase goes to "Snapshotted"
	// +optional
	SnapshotID string `json:"snapshotID,omitempty"`

	// Metadata for the snapshotted object
	Metadata []byte `json:"metadata,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

/*
 Snapshot is used to request that a snapshot is taken.  It is not used to manage the inventory of snapshots and does not
 need to exist in order to clone the resource from a snapshot
*/
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:subresource:status
type Snapshot struct {
	// TypeMeta is the metadata for the resource, like kind and apiversion
	meta_v1.TypeMeta `json:",inline"`

	// +optional
	meta_v1.ObjectMeta `json:"metadata,omitempty"`

	// Spec is the custom resource spec
	Spec SnapshotSpec `json:"spec"`

	// Current status of the snapshot operation
	// +optional
	Status SnapshotStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SnapshotList is a list of Snapshot resources
type SnapshotList struct {
	meta_v1.TypeMeta `json:",inline"`

	// +optional
	meta_v1.ListMeta `json:"metadata,omitempty"`

	Items []Snapshot `json:"items"`
}

/*
CloneFromSnapshotSpec specifies an object to be cloned from a snapshot ID.  The Metadata may be overridden, the format
of the metadata is object specific.  APIGroup and Kind specify the type of object to create.
*/
type CloneFromSnapshotSpec struct {
	SnapshotID string `json:"snapshotID"`

	// +optional - if set, this overrides metadata that was stored in the snapshot
	Metadata []byte `json:"metadata,omitempty"`

	// APIGroup of the resource being created
	APIGroup *string `json:"apiGroup"`
	// Kind is the type of resource being created
	Kind string `json:"kind"`

	// The backup repository to retrieve the snapshot from.  The namespace the Snapshot/PVC lives in must have access to the repository
	BackupRepository string `json:"backpRepository"`

	// SnapshotCancel indicates request to cancel ongoing snapshot.  SnapshotCancel can be set at anytime before
	// the snapshot reaches a terminal phase.  If the snapshot has reached a terminal phase
	CloneCancel bool `json:"cloneCancel"`
}

/*
  ClonePhase represents the lifecycle phase of a Clone.
  New - No work yet, next phase is InProgress
  InProgress - snapshot being taken
  Completed - new object has been created
  Failed - end state, clone failed, no new object was created
  Canceling - when the Clone flag is set, if the Clone has not already moved into a terminal state, the
              status will move to Canceling.  The object that was being created will be removed
 Canceled - the Clone was canceled, no new object was created
*/
type ClonePhase string

const (
	ClonePhaseNew        ClonePhase = "New"
	ClonePhaseInProgress ClonePhase = "InProgress"
	ClonePhaseCompleted  ClonePhase = "Completed"
	ClonePhaseRetry      ClonePhase = "Retry"
	ClonePhaseFailed     ClonePhase = "Failed"
	ClonePhaseCanceling  ClonePhase = "Canceling"
	ClonePhaseCanceled   ClonePhase = "Canceled"
)

type CloneStatus struct {
	Phase ClonePhase `json:"phase"`

	Message string `json:"message"`
	// The handle of the resource that was cloned from the snapshot
	// +optional
	ResourceHandle core_v1.TypedLocalObjectReference `json:"resourceHandle"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

/*
 CloneFromSnapshot is used to create a new resource (typically a PVC) from a snapshot.  Once the Snapshot's Phase has
 moved to Snapshotted it is valid to create a new resource from the snapshot ID
*/
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:subresource:status
type CloneFromSnapshot struct {
	// TypeMeta is the metadata for the resource, like kind and apiversion
	meta_v1.TypeMeta `json:",inline"`

	// +optional
	meta_v1.ObjectMeta `json:"metadata,omitempty"`

	Spec CloneFromSnapshotSpec `json:"spec"`

	Status CloneStatus `json:"status"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// CloneFromSnapshotList is a list of CloneFromSnapshot resources
type CloneFromSnapshotList struct {
	meta_v1.TypeMeta `json:",inline"`

	// +optional
	meta_v1.ListMeta `json:"metadata,omitempty"`

	Items []CloneFromSnapshot `json:"items"`
}
