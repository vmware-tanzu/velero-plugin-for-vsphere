package builder

import (
	backupdriverv1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/backupdriver/v1"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/constants"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BackupRepositoryBuilder builds BackupRepository objects.
type BackupRepositoryBuilder struct {
	object *backupdriverv1.BackupRepository
}

func ForBackupRepository(name string) *BackupRepositoryBuilder {
	return &BackupRepositoryBuilder{
		object: &backupdriverv1.BackupRepository{
			TypeMeta: metav1.TypeMeta{
				APIVersion: backupdriverv1.SchemeGroupVersion.String(),
				Kind:       "BackupRepository",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
		},
	}
}

// Result returns the built BackupRepository.
func (b *BackupRepositoryBuilder) Result() *backupdriverv1.BackupRepository {
	return b.object
}

// AllowedNamespaces sets the allowed namespaces for the backup repository.
func (b *BackupRepositoryBuilder) AllowedNamespaces(allowedNamespaces []string) *BackupRepositoryBuilder {
	b.object.AllowedNamespaces = make([]string, len(allowedNamespaces))
	copy(b.object.AllowedNamespaces, allowedNamespaces)
	return b
}

// RepositoryDriver sets the repository driver the backup repository. Currently only s3 is supported.
func (b *BackupRepositoryBuilder) RepositoryDriver() *BackupRepositoryBuilder {
	// TODO: Change this when other drivers are supported.
	b.object.RepositoryDriver = constants.S3RepositoryDriver
	return b
}

// RepositoryParameters sets the parameters for the backup repository, these mostly include the credentials.
func (b *BackupRepositoryBuilder) RepositoryParameters(repositoryParameters map[string]string) *BackupRepositoryBuilder {
	b.object.RepositoryParameters = repositoryParameters
	return b
}

// BackupRepositoryClaim sets the name of the backup repository claim for this specific backup repository.
func (b *BackupRepositoryBuilder) BackupRepositoryClaim(backupRepositoryClaimName string) *BackupRepositoryBuilder {
	b.object.BackupRepositoryClaim = backupRepositoryClaimName
	return b
}

// SvcBackupRepositoryName sets the name of the supervisor backup repository
// corresponding to this specific backup repository, if it is not empty.
// This is available only for guest clusters.
func (b *BackupRepositoryBuilder) SvcBackupRepositoryName(svcBackupRepositoryName string) *BackupRepositoryBuilder {
	if svcBackupRepositoryName != "" {
		b.object.SvcBackupRepositoryName = svcBackupRepositoryName
	}
	return b
}
