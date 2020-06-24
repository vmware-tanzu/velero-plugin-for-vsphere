package backup_driver_client

import "context"

type BackupRepository struct {
	backupRepository string
}

/*
This will either create a new BackupRepositoryClaim record or update/use an existing BackupRepositoryClaim record.  In
either case, it does not return until the BackupRepository is assigned and ready to use or the context is canceled
(in which case it will return an error).
 */
func ClaimBackupRepository(ctx context.Context, repositoryDriver string,
	repositoryParameters map[string]string,
	allowedNamespaces []string) (*BackupRepository, error) {
	return nil, nil
}

func DeleteBackupRepository(ctx context.Context, deleteRepository *BackupRepository) error {
	return nil
}
