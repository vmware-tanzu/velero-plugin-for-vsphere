package v1

import meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

/*
 BackupRepository is a cluster-scoped resource.  It is controlled by the Backup Driver and referenced by
 Snapshot, CloneFromSnapshot and Delete.  The BackupRespository resource contains the credential for a backup repository.
 The RepositoryDriver defines the driver that will be used to talk to the repository
 Only Snapshot,etc. CRs from namespaces that are listed in AllowedNamespaces will be acted on, if the namespace is
 not in AllowedNamespaces the operation will fail.
*/
type BackupRepository struct {
	// TypeMeta is the metadata for the resource, like kind and apiversion
	meta_v1.TypeMeta `json:",inline"`

	// +optional
	meta_v1.ObjectMeta `json:"metadata,omitempty"`

	AllowedNamespaces []string `json:"allowedNamespaces"`

	RepositoryDriver      string            `json:"repositoryDriver"`
	RepositoryParameters  map[string]string `json:"repopsitoryParameters"`
	BackupRepositoryClaim string            `json:backupRepositoryClaim`
}

/*
 BackupRepositoryClaim is used to define/access a BackupRepository.  A new BackupRepository will be created
 with the RepositoryDriver, Credential and AllowedNamespaces will either be the namespace that the BackupRepositorySpec
 was created in or the AllowedNamespaces specified in the BackupRepositorySpec.  The BackupRepository field will
 be updated with the name of the BackupRepository created.
*/
type BackupRepositoryClaim struct {
	// TypeMeta is the metadata for the resource, like kind and apiversion
	meta_v1.TypeMeta `json:",inline"`

	// +optional
	meta_v1.ObjectMeta `json:"metadata,omitempty"`

	// +optional
	AllowedNamespaces []string `json:"allowedNamespaces, omitempty"`

	RepositoryDriver     string            `json:"repositoryDriver"`
	RepositoryParameters map[string]string `json:"repopsitoryParameters"`

	// +optional
	BackupRepository string `json: "backupRepository, omitempty"`
}
