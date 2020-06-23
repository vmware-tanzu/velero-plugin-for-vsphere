package v1

import meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

/*
 BackupRepository is a cluster-scoped resource.  It is controlled by the Backup Driver and referenced by
 Snapshot, CloneFromSnapshot and Delete.  The BackupRespository resource contains the credential for a backup repository.
 The RepositoryDriver defines the driver that will be used to talk to the repository
 Only Snapshot,etc. CRs from namespaces that are listed in AllowedNamespaces will be acted on, if the namespace is
 not in AllowedNamespaces the operation will fail.
*/
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
type BackupRepository struct {
	// TypeMeta is the metadata for the resource, like kind and apiversion
	meta_v1.TypeMeta `json:",inline"`

	// +optional
	meta_v1.ObjectMeta `json:"metadata,omitempty"`

	AllowedNamespaces []string `json:"allowedNamespaces"`

	RepositoryDriver      string            `json:"repositoryDriver"`
	RepositoryParameters  map[string]string `json:"repopsitoryParameters"`
	BackupRepositoryClaim string            `json:"backupRepositoryClaim"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// BackupRepositoryList is a list of BackupRepository resources
type BackupRepositoryList struct {
	meta_v1.TypeMeta `json:",inline"`

	// +optional
	meta_v1.ListMeta `json:"metadata,omitempty"`

	Items []BackupRepository `json:"items"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

/*
 BackupRepositoryClaim is used to define/access a BackupRepository.  A new BackupRepository will be created
 with the RepositoryDriver, Credential and AllowedNamespaces will either be the namespace that the BackupRepositorySpec
 was created in or the AllowedNamespaces specified in the BackupRepositorySpec.  The BackupRepository field will
 be updated with the name of the BackupRepository created.
*/
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced
type BackupRepositoryClaim struct {
	// TypeMeta is the metadata for the resource, like kind and apiversion
	meta_v1.TypeMeta `json:",inline"`

	// +optional
	meta_v1.ObjectMeta `json:"metadata,omitempty"`

	// +optional
	AllowedNamespaces []string `json:"allowedNamespaces,omitempty"`

	RepositoryDriver     string            `json:"repositoryDriver"`
	RepositoryParameters map[string]string `json:"repopsitoryParameters"`

	// +optional
	BackupRepository string `json:"backupRepository,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// BackupRepositoryClaimList is a list of BackupRepositoryClaim resources
type BackupRepositoryClaimList struct {
	meta_v1.TypeMeta `json:",inline"`

	// +optional
	meta_v1.ListMeta `json:"metadata,omitempty"`

	Items []BackupRepositoryClaim `json:"items"`
}
