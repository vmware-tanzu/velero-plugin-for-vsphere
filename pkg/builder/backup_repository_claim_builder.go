package builder

import (
	backupdriverv1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/backupdriver/v1alpha1"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/constants"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BackupRepositoryClaimBuilder builds BackupRepositoryClaim objects.
type BackupRepositoryClaimBuilder struct {
	object *backupdriverv1.BackupRepositoryClaim
}

func ForBackupRepositoryClaim(ns, name string) *BackupRepositoryClaimBuilder {
	return &BackupRepositoryClaimBuilder{
		object: &backupdriverv1.BackupRepositoryClaim{
			TypeMeta: metav1.TypeMeta{
				APIVersion: backupdriverv1.SchemeGroupVersion.String(),
				Kind:       "BackupRepositoryClaim",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ns,
				Labels:    utils.AppendVeleroExcludeLabels(nil),
			},
		},
	}
}

// Result returns the built BackupRepositoryClaim.
func (b *BackupRepositoryClaimBuilder) Result() *backupdriverv1.BackupRepositoryClaim {
	return b.object
}

// AllowedNamespaces sets the allowed namespaces for the backup repository claim.
func (b *BackupRepositoryClaimBuilder) AllowedNamespaces(allowedNamespaces []string) *BackupRepositoryClaimBuilder {
	b.object.AllowedNamespaces = make([]string, len(allowedNamespaces))
	copy(b.object.AllowedNamespaces, allowedNamespaces)
	return b
}

// RepositoryDriver sets the repository driver the backup repository claim. Currently only s3 is supported.
func (b *BackupRepositoryClaimBuilder) RepositoryDriver() *BackupRepositoryClaimBuilder {
	// TODO: Change this when other drivers are supported.
	b.object.RepositoryDriver = constants.S3RepositoryDriver
	return b
}

// RepositoryParameters sets the parameters for the backup repository claim, these mostly include the credentials.
func (b *BackupRepositoryClaimBuilder) RepositoryParameters(repositoryParameters map[string]string) *BackupRepositoryClaimBuilder {
	b.object.RepositoryParameters = repositoryParameters
	return b
}

// BackupRepository sets the name of the backup repository for this specific backup repository claim.
func (b *BackupRepositoryClaimBuilder) BackupRepository(backupRepositoryName string) *BackupRepositoryClaimBuilder {
	b.object.BackupRepository = backupRepositoryName
	return b
}
