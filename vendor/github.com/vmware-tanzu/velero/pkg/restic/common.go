/*
Copyright 2018 the Velero contributors.

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

package restic

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	corev1listers "k8s.io/client-go/listers/core/v1"

	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	velerov1listers "github.com/vmware-tanzu/velero/pkg/generated/listers/velero/v1"
	"github.com/vmware-tanzu/velero/pkg/label"
	"github.com/vmware-tanzu/velero/pkg/util/filesystem"
)

const (
	// DaemonSet is the name of the Velero restic daemonset.
	DaemonSet = "restic"

	// InitContainer is the name of the init container added
	// to workload pods to help with restores.
	InitContainer = "restic-wait"

	// DefaultMaintenanceFrequency is the default time interval
	// at which restic prune is run.
	DefaultMaintenanceFrequency = 7 * 24 * time.Hour

	// PVCNameAnnotation is the key for the annotation added to
	// pod volume backups when they're for a PVC.
	PVCNameAnnotation = "velero.io/pvc-name"

	// Deprecated.
	//
	// TODO(2.0): remove
	podAnnotationPrefix = "snapshot.velero.io/"

	volumesToBackupAnnotation = "backup.velero.io/backup-volumes"
)

// getPodSnapshotAnnotations returns a map, of volume name -> snapshot id,
// of all restic snapshots for this pod.
// TODO(2.0) to remove
// Deprecated: we will stop using pod annotations to record restic snapshot IDs after they're taken,
// therefore we won't need to check if these annotations exist.
func getPodSnapshotAnnotations(obj metav1.Object) map[string]string {
	var res map[string]string

	insertSafe := func(k, v string) {
		if res == nil {
			res = make(map[string]string)
		}
		res[k] = v
	}

	for k, v := range obj.GetAnnotations() {
		if strings.HasPrefix(k, podAnnotationPrefix) {
			insertSafe(k[len(podAnnotationPrefix):], v)
		}
	}

	return res
}

// GetVolumeBackupsForPod returns a map, of volume name -> snapshot id,
// of the PodVolumeBackups that exist for the provided pod.
func GetVolumeBackupsForPod(podVolumeBackups []*velerov1api.PodVolumeBackup, pod metav1.Object) map[string]string {
	volumes := make(map[string]string)

	for _, pvb := range podVolumeBackups {
		if pod.GetName() != pvb.Spec.Pod.Name {
			continue
		}

		// skip PVBs without a snapshot ID since there's nothing
		// to restore (they could be failed, or for empty volumes).
		if pvb.Status.SnapshotID == "" {
			continue
		}

		volumes[pvb.Spec.Volume] = pvb.Status.SnapshotID
	}

	if len(volumes) > 0 {
		return volumes
	}

	return getPodSnapshotAnnotations(pod)
}

// GetVolumesToBackup returns a list of volume names to backup for
// the provided pod.
func GetVolumesToBackup(obj metav1.Object) []string {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		return nil
	}

	backupsValue := annotations[volumesToBackupAnnotation]
	if backupsValue == "" {
		return nil
	}

	return strings.Split(backupsValue, ",")
}

// SnapshotIdentifier uniquely identifies a restic snapshot
// taken by Velero.
type SnapshotIdentifier struct {
	// VolumeNamespace is the namespace of the pod/volume that
	// the restic snapshot is for.
	VolumeNamespace string

	// BackupStorageLocation is the backup's storage location
	// name.
	BackupStorageLocation string

	// SnapshotID is the short ID of the restic snapshot.
	SnapshotID string
}

// GetSnapshotsInBackup returns a list of all restic snapshot ids associated with
// a given Velero backup.
func GetSnapshotsInBackup(backup *velerov1api.Backup, podVolumeBackupLister velerov1listers.PodVolumeBackupLister) ([]SnapshotIdentifier, error) {
	selector := labels.Set(map[string]string{
		velerov1api.BackupNameLabel: label.GetValidName(backup.Name),
	}).AsSelector()

	podVolumeBackups, err := podVolumeBackupLister.List(selector)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	var res []SnapshotIdentifier
	for _, item := range podVolumeBackups {
		if item.Status.SnapshotID == "" {
			continue
		}
		res = append(res, SnapshotIdentifier{
			VolumeNamespace:       item.Spec.Pod.Namespace,
			BackupStorageLocation: backup.Spec.StorageLocation,
			SnapshotID:            item.Status.SnapshotID,
		})
	}

	return res, nil
}

// TempCredentialsFile creates a temp file containing a restic
// encryption key for the given repo and returns its path. The
// caller should generally call os.Remove() to remove the file
// when done with it.
func TempCredentialsFile(secretLister corev1listers.SecretLister, veleroNamespace, repoName string, fs filesystem.Interface) (string, error) {
	secretGetter := NewListerSecretGetter(secretLister)

	// For now, all restic repos share the same key so we don't need the repoName to fetch it.
	// When we move to full-backup encryption, we'll likely have a separate key per restic repo
	// (all within the Velero server's namespace) so GetRepositoryKey will need to take the repo
	// name as an argument as well.
	repoKey, err := GetRepositoryKey(secretGetter, veleroNamespace)
	if err != nil {
		return "", err
	}

	file, err := fs.TempFile("", fmt.Sprintf("%s-%s", CredentialsSecretName, repoName))
	if err != nil {
		return "", errors.WithStack(err)
	}

	if _, err := file.Write(repoKey); err != nil {
		// nothing we can do about an error closing the file here, and we're
		// already returning an error about the write failing.
		file.Close()
		return "", errors.WithStack(err)
	}

	name := file.Name()

	if err := file.Close(); err != nil {
		return "", errors.WithStack(err)
	}

	return name, nil
}

// NewPodVolumeBackupListOptions creates a ListOptions with a label selector configured to
// find PodVolumeBackups for the backup identified by name.
func NewPodVolumeBackupListOptions(name string) metav1.ListOptions {
	return metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", velerov1api.BackupNameLabel, label.GetValidName(name)),
	}
}

// NewPodVolumeRestoreListOptions creates a ListOptions with a label selector configured to
// find PodVolumeRestores for the restore identified by name.
func NewPodVolumeRestoreListOptions(name string) metav1.ListOptions {
	return metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", velerov1api.RestoreNameLabel, label.GetValidName(name)),
	}
}

// AzureCmdEnv returns a list of environment variables (in the format var=val) that
// should be used when running a restic command for an Azure backend. This list is
// the current environment, plus the Azure-specific variables restic needs, namely
// a storage account name and key.
func AzureCmdEnv(backupLocationLister velerov1listers.BackupStorageLocationLister, namespace, backupLocation string) ([]string, error) {
	loc, err := backupLocationLister.BackupStorageLocations(namespace).Get(backupLocation)
	if err != nil {
		return nil, errors.Wrap(err, "error getting backup storage location")
	}

	azureVars, err := getResticEnvVars(loc.Spec.Config)
	if err != nil {
		return nil, errors.Wrap(err, "error getting azure restic env vars")
	}

	env := os.Environ()
	for k, v := range azureVars {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	return env, nil
}
