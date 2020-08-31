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

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/tools/cache"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	jsonpatch "github.com/evanphx/json-patch"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/vmware-tanzu/astrolabe/pkg/astrolabe"
	"github.com/vmware-tanzu/astrolabe/pkg/ivd"
	"github.com/vmware-tanzu/astrolabe/pkg/s3repository"
	backupdriverapi "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/backupdriver/v1"
	pluginv1api "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/veleroplugin/v1"
	plugin_clientset "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned"
	backupdriverv1client "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned/typed/backupdriver/v1"
	pluginv1client "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned/typed/veleroplugin/v1"
	v1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"github.com/vmware-tanzu/velero/pkg/generated/clientset/versioned"
	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

/*
 * In the CSI setup, VC credential is stored as a secret
 * under the kube-system namespace.
 */
func RetrieveVcConfigSecret(params map[string]interface{}, config *rest.Config, logger logrus.FieldLogger) error {
	var err error // Declare here to avoid shadowing on config using := with rest.InClusterConfig
	if config == nil {
		config, err = rest.InClusterConfig()
		if err != nil {
			logger.WithError(err).Errorf("Failed to get k8s inClusterConfig")
			return errors.Wrap(err, "Could not retrieve in-cluster config")
		}
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		logger.WithError(err).Errorf("Failed to get k8s clientset from the given config: %v", config)
		return err
	}

	// Get the cluster flavor
	var ns, secretData string
	clusterFlavor, err := GetClusterFlavor(config)
	if clusterFlavor == TkgGuest || clusterFlavor == Unknown {
		logger.Errorf("RetrieveVcConfigSecret: Cannot retrieve VC secret in cluster flavor %s", clusterFlavor)
		return errors.New("RetrieveVcConfigSecret: Cannot retrieve VC secret")
	} else if clusterFlavor == Supervisor {
		ns = VCSecretNsSupervisor
		secretData = VCSecretDataSupervisor
	} else {
		ns = VCSecretNs
		secretData = VCSecretData
	}

	secretApis := clientset.CoreV1().Secrets(ns)
	vsphere_secrets := []string{VCSecret, VCSecretTKG}
	var secret *k8sv1.Secret
	for _, vsphere_secret := range vsphere_secrets {
		secret, err = secretApis.Get(context.TODO(), vsphere_secret, metav1.GetOptions{})
		if err == nil {
			logger.Infof("Retrieved k8s secret, %s", vsphere_secret)
			break
		}
		logger.WithError(err).Infof("Skipping k8s secret %s as it does not exist", vsphere_secret)
	}

	// No valid secret found.
	if err != nil {
		logger.WithError(err).Errorf("Failed to get k8s secret, %s", vsphere_secrets)
		return err
	}

	sEnc := string(secret.Data[secretData])
	lines := strings.Split(sEnc, "\n")

	ParseLines(lines, params, logger)

	// If port is missing, add an entry in the params to use the standard https port
	if _, ok := params["port"]; !ok {
		params["port"] = DefaultVCenterPort
	}

	return nil
}

func ParseLines(lines []string, params map[string]interface{}, logger logrus.FieldLogger) {
	for _, line := range lines {
		if strings.Contains(line, "VirtualCenter") {
			parts := strings.Split(line, "\"")
			params["VirtualCenter"] = parts[1]
		} else if strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			// Skip the quotes in the value if present
			unquotedValue, err := strconv.Unquote(string(value))
			if err != nil {
				logger.WithError(err).Errorf("Failed to unquote value %v for key %v. Just store the original value string", value, key)
				params[key] = string(value)
				continue
			}
			params[key] = unquotedValue
		}
	}
}

func RetrieveParamsFromBSL(repositoryParams map[string]string, bslName string, config *rest.Config,
	logger logrus.FieldLogger) error {
	s3RepoParams := make(map[string]interface{})
	err := RetrieveVSLFromVeleroBSLs(s3RepoParams, bslName, config, logger)
	if err != nil {
		return err
	}
	//Translate s3RepoParams to repositoryParams.
	for key, val := range s3RepoParams {
		paramValue, ok := val.(string)
		if !ok {
			return errors.Errorf("Failed to translate s3 repository parameter value: %v", val)
		}
		repositoryParams[key] = paramValue
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return errors.Wrap(err, "Failed to retrieve the k8s clientset")
	}

	veleroNs, exist := os.LookupEnv("VELERO_NAMESPACE")
	if !exist {
		logger.Errorf("RetrieveParamsFromBSL: Failed to lookup the env variable for velero namespace")
		return err
	}

	secretsClient := clientset.CoreV1().Secrets(veleroNs)
	secret, err := secretsClient.Get(context.TODO(), CloudCredentialSecretName, metav1.GetOptions{})
	if err != nil {
		logger.Errorf("RetrieveParamsFromBSL: Failed to retrieve the Secret for %s", CloudCredentialSecretName)
		return err
	}

	for _, value := range secret.Data {
		tmpfile, err := ioutil.TempFile("", "temp-aws-cred")
		if err != nil {
			return errors.Wrap(err, "Failed to create temp file to extract aws credentials")
		}
		// Cleanup
		defer os.Remove(tmpfile.Name())

		// Writing the encoded value into into a temporary file.
		// The file is in a non-standard format, aws APIs recognize the format.
		if _, err := tmpfile.Write(value); err != nil {
			return errors.Wrap(err, "Failed to write aws credentials into temp file.")
		}
		if err := tmpfile.Close(); err != nil {
			return errors.Wrap(err, "Failed to close into temp file.")
		}
		// Extract the right set of credentials based on the profile extracted from BSL.
		awsCredentials := credentials.NewSharedCredentials(tmpfile.Name(), repositoryParams["profile"])
		awsPlainCred, err := awsCredentials.Get()
		if err != nil {
			logger.Errorf("RetrieveParamsFromBSL: Failed to extract credentials for profile :%s", repositoryParams["profile"])
			return err
		}
		repositoryParams[AWS_ACCESS_KEY_ID] = awsPlainCred.AccessKeyID
		repositoryParams[AWS_SECRET_ACCESS_KEY] = awsPlainCred.SecretAccessKey
		logger.Infof("Successfully retrieved AWS credentials for the BackupStorageLocation.")
		//Breaking since its expected to have only one kv pair for the secret data.
		break
	}

	return nil
}

/*
 * Retrieve the Volume Snapshot Location(VSL) as the remote storage location
 * for the data manager component in plugin from the Backup Storage Locations(BSLs)
 * of Velero. It will always pick up the first available one.
 */
func RetrieveVSLFromVeleroBSLs(params map[string]interface{}, bslName string, config *rest.Config, logger logrus.FieldLogger) error {
	var err error // Declare here to avoid shadowing on config using := with rest.InClusterConfig
	if config == nil {
		config, err = rest.InClusterConfig()
		if err != nil {
			return errors.Wrap(err, "Could not retrieve in-cluster config")
		}
	}

	veleroClient, err := versioned.NewForConfig(config)
	if err != nil {
		return err
	}

	veleroNs, exist := os.LookupEnv("VELERO_NAMESPACE")
	if !exist {
		logger.Errorf("RetrieveVSLFromVeleroBSLs: Failed to lookup the env variable for velero namespace")
		return err
	}

	var backupStorageLocation *v1.BackupStorageLocation
	backupStorageLocation, err = veleroClient.VeleroV1().BackupStorageLocations(veleroNs).
		Get(context.TODO(), bslName, metav1.GetOptions{})

	if err != nil {
		logger.WithError(err).Infof("RetrieveVSLFromVeleroBSLs: Failed to get Velero %s backup storage location,"+
			" attempting to find available BSL", bslName)
		backupStorageLocationList, err := veleroClient.VeleroV1().BackupStorageLocations(veleroNs).List(context.TODO(), metav1.ListOptions{})
		if err != nil || len(backupStorageLocationList.Items) <= 0 {
			logger.WithError(err).Errorf("RetrieveVSLFromVeleroBSLs: Failed to list Velero default backup storage location")
			return err
		}
		// Select the first valid BackupStorageLocation from the list if there is no default BackupStorageLocation.
		logger.Infof("RetrieveVSLFromVeleroBSLs: Picked up the first valid BackupStorageLocation from the BackupStorageLocationList")
		for _, item := range backupStorageLocationList.Items {
			backupStorageLocation = &item
			provider := strings.ToLower(item.Spec.Provider)
			if provider != "aws" {
				logger.Warnf("RetrieveVSLFromVeleroBSLs: Object store providers, %v, other than AWS are not supported for the moment", provider)
				continue
			}
			region := backupStorageLocation.Spec.Config["region"]
			if region == "" {
				logger.Warnf("RetrieveVSLFromVeleroBSLs: The region field is missing in the Backup Storage Location. Skiping.")
				continue
			}
		}
	}

	if backupStorageLocation == nil {
		return errors.New("RetrieveVSLFromVeleroBSLs: No valid Backup Storage Location can be retrieved")
	}

	params["region"] = backupStorageLocation.Spec.Config["region"]
	params["bucket"] = backupStorageLocation.Spec.ObjectStorage.Bucket
	params["s3ForcePathStyle"] = backupStorageLocation.Spec.Config["s3ForcePathStyle"]
	params["s3Url"] = backupStorageLocation.Spec.Config["s3Url"]
	params["profile"] = backupStorageLocation.Spec.Config["profile"]

	return nil
}

func GetIVDPETMFromParamsMap(params map[string]interface{}, logger logrus.FieldLogger) (*ivd.IVDProtectedEntityTypeManager, error) {
	// Largely a dummy s3Config - s3Config is to enable access to astrolabe objects via S3 which we don't support from
	// here
	s3Config := astrolabe.S3Config{
		Port:      0,
		Host:      nil,
		AccessKey: "",
		Secret:    "",
		Prefix:    "",
		URLBase:   "VOID_URL",
	}

	ivdPETM, err := ivd.NewIVDProtectedEntityTypeManagerFromConfig(params, s3Config, logger)
	if err != nil {
		logger.WithError(err).Errorf("Error at creating new IVD PETM from vc params: %v, s3Config: %v",
			params, s3Config)
		return nil, err
	}

	return ivdPETM, nil
}

func GetS3PETMFromParamsMap(params map[string]interface{}, logger logrus.FieldLogger) (*s3repository.ProtectedEntityTypeManager, error) {
	serviceType := "ivd"
	region, ok := GetStringFromParamsMap(params, "region", logger)
	if !ok {
		return nil, errors.New("Missing region param, cannot initialize S3 PETM")
	}

	bucket, ok := GetStringFromParamsMap(params, "bucket", logger)
	if !ok {
		return nil, errors.New("Missing bucket param, cannot initialize S3 PETM")
	}

	// If the credentials are explicitly provided in params, use it.
	// else let aws API pick the default credential provider.
	var sess *session.Session
	if _, ok := params[AWS_ACCESS_KEY_ID]; ok {
		s3AccessKeyId, ok := GetStringFromParamsMap(params, AWS_ACCESS_KEY_ID, logger)
		if !ok {
			return nil, errors.New("Failed to retrieve S3 Access Key.")
		}
		s3SecretAccessKey, ok := GetStringFromParamsMap(params, AWS_SECRET_ACCESS_KEY, logger)
		if !ok {
			return nil, errors.New("Failed to retrieve S3 Secret Access Key.")
		}
		logger.Infof("Using explicitly found credentials for S3 repository access.")
		sess = session.Must(session.NewSession(&aws.Config{
			Region:      aws.String(region),
			Credentials: credentials.NewStaticCredentials(s3AccessKeyId, s3SecretAccessKey, ""),
		}))
	} else {
		sess = session.Must(session.NewSession(&aws.Config{
			Region: aws.String(region),
		}))
	}

	s3Url, ok := GetStringFromParamsMap(params, "s3Url", logger)
	if ok {
		sess.Config.Endpoint = aws.String(s3Url)
	}

	pathStyle, ok := GetStringFromParamsMap(params, "s3ForcePathStyle", logger)
	if ok {
		if GetBool(pathStyle, false) {
			sess.Config.S3ForcePathStyle = aws.Bool(true)
			logger.Infof("Got %s for s3ForcePathStyle, setting s3ForcePathStyle to true", pathStyle)
		} else {
			sess.Config.S3ForcePathStyle = aws.Bool(false)
			logger.Infof("Got %s for s3ForcePathStyle, setting s3ForcePathStyle to false", pathStyle)
		}
	}

	prefix, ok := params["prefix"].(string)
	if !ok {
		prefix = DefaultS3RepoPrefix
	}
	s3PETM, err := s3repository.NewS3RepositoryProtectedEntityTypeManager(serviceType, *sess, bucket, prefix, logger)
	if err != nil {
		logger.WithError(err).Errorf("Error at creating new S3 PETM from serviceType: %s, region: %s, bucket: %s",
			serviceType, region, bucket)
		return nil, err
	}

	return s3PETM, nil
}

func GetStringFromParamsMap(params map[string]interface{}, key string, logger logrus.FieldLogger) (value string, ok bool) {
	valueIF, ok := params[key]
	if ok {
		value, ok := valueIF.(string)
		if !ok {
			logger.Errorf("Value for params key %s is not a string", key)
		}
		return value, ok
	} else {
		logger.Errorf("No such key %s in params map", key)
		return "", ok
	}
}

func GetBool(str string, defValue bool) bool {
	if str == "" {
		return defValue
	}

	res, err := strconv.ParseBool(str)
	if err != nil {
		res = defValue
		err = nil
	}

	return res
}

type NotFoundError struct {
	errMsg string
}

func (this NotFoundError) Error() string {
	return this.errMsg
}

func NewNotFoundError(errMsg string) NotFoundError {
	err := NotFoundError{
		errMsg: errMsg,
	}
	return err
}

func RetrievePodNodesByVolumeId(volumeId string) (string, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return "", err
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return "", err
	}

	pvList, err := clientset.CoreV1().PersistentVolumes().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return "", err
	}

	var claimRefName, claimRefNs string
	for _, pv := range pvList.Items {
		if pv.Spec.CSI != nil && pv.Spec.CSI.VolumeHandle == volumeId {
			claimRefName = pv.Spec.ClaimRef.Name
			claimRefNs = pv.Spec.ClaimRef.Namespace
			break
		}
	}
	if claimRefNs == "" || claimRefName == "" {
		errMsg := fmt.Sprintf("Failed to retrieve the PV with the expected volume ID, %v", volumeId)
		return "", NewNotFoundError(errMsg)
	}

	podList, err := clientset.CoreV1().Pods(claimRefNs).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return "", err
	}

	// Assume the PV is specified with RWO (ReadWriteOnce)
	var nodeName string
	for _, pod := range podList.Items {
		for _, volume := range pod.Spec.Volumes {
			if volume.PersistentVolumeClaim == nil {
				continue
			}
			if volume.PersistentVolumeClaim.ClaimName == claimRefName {
				nodeName = pod.Spec.NodeName
				break
			}
		}
		if nodeName != "" {
			break
		}
	}

	if nodeName == "" {
		errMsg := fmt.Sprintf("Failed to retrieve pod that claim the PV, %v", volumeId)
		return "", NewNotFoundError(errMsg)
	}

	return nodeName, nil
}

func PatchUpload(req *pluginv1api.Upload, mutate func(*pluginv1api.Upload), uploadClient pluginv1client.UploadInterface, logger logrus.FieldLogger) (*pluginv1api.Upload, error) {

	// Record original json
	oldData, err := json.Marshal(req)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to marshall original Upload")
	}

	// Mutate
	mutate(req)

	// Record new json
	newData, err := json.Marshal(req)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to marshall updated Upload")
	}

	patchBytes, err := jsonpatch.CreateMergePatch(oldData, newData)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to creat json merge patch for Upload")
	}

	req, err = uploadClient.Patch(context.TODO(), req.Name, types.MergePatchType, patchBytes, metav1.PatchOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to patch Upload")
	}
	return req, nil
}

func PatchBackupRepositoryClaim(req *backupdriverapi.BackupRepositoryClaim,
	mutate func(*backupdriverapi.BackupRepositoryClaim),
	backupRepoClaimClient backupdriverv1client.BackupRepositoryClaimInterface) (*backupdriverapi.BackupRepositoryClaim, error) {

	// Record original json
	oldData, err := json.Marshal(req)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to marshall original BackupRepositoryClaim")
	}

	// Mutate
	mutate(req)

	// Record new json
	newData, err := json.Marshal(req)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to marshall updated BackupRepositoryClaim")
	}

	patchBytes, err := jsonpatch.CreateMergePatch(oldData, newData)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to creat json merge patch for BackupRepositoryClaim")
	}

	req, err = backupRepoClaimClient.Patch(context.TODO(), req.Name, types.MergePatchType, patchBytes, metav1.PatchOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to patch BackupRepositoryClaim")
	}
	return req, nil
}

func PatchBackupRepository(req *backupdriverapi.BackupRepository,
	mutate func(*backupdriverapi.BackupRepository),
	backupRepoClient backupdriverv1client.BackupRepositoryInterface) (*backupdriverapi.BackupRepository, error) {

	// Record original json
	oldData, err := json.Marshal(req)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to marshall original BackupRepository")
	}

	// Mutate
	mutate(req)

	// Record new json
	newData, err := json.Marshal(req)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to marshall updated BackupRepository")
	}

	patchBytes, err := jsonpatch.CreateMergePatch(oldData, newData)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to creat json merge patch for BackupRepository")
	}

	req, err = backupRepoClient.Patch(context.TODO(), req.Name, types.MergePatchType, patchBytes, metav1.PatchOptions{})
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to patch BackupRepository")
	}
	return req, nil
}

// Check the cluster flavor that the plugin is deployed in
func GetClusterFlavor(config *rest.Config) (ClusterFlavor, error) {
	var err error
	if config == nil {
		config, err = rest.InClusterConfig()
		if err != nil {
			return Unknown, err
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return Unknown, err
	}

	// Direct vSphere deployment.
	// Check if vSphere secret is available in appropriate namespace.
	ns := VCSecretNs
	secretApis := clientset.CoreV1().Secrets(ns)
	vsphere_secrets := []string{VCSecret, VCSecretTKG}
	for _, vsphere_secret := range vsphere_secrets {
		_, err := secretApis.Get(context.TODO(), vsphere_secret, metav1.GetOptions{})
		if err == nil {
			return VSphere, nil
		}
	}

	// Check if in supervisor.
	// Check if vSphere secret is available in appropriate namespace.
	ns = VCSecretNsSupervisor
	secretApis = clientset.CoreV1().Secrets(ns)
	_, err = secretApis.Get(context.TODO(), VCSecret, metav1.GetOptions{})
	if err == nil {
		return Supervisor, nil
	}

	// Check if in guest cluster.
	// Check for the supervisor service in the guest cluster.
	serviceApi := clientset.CoreV1().Services("default")
	_, err = serviceApi.Get(context.TODO(), TkgSupervisorService, metav1.GetOptions{})
	if err == nil {
		return TkgGuest, nil
	}

	// Did not match any search criteria. Unknown cluster flavor.
	return Unknown, errors.New("GetClusterFlavor: Failed to identify cluster flavor")
}

func GetRepositoryFromBackupRepository(backupRepository *backupdriverapi.BackupRepository, logger logrus.FieldLogger) (*s3repository.ProtectedEntityTypeManager, error) {
	switch backupRepository.RepositoryDriver {
	case S3RepositoryDriver:
		params := make(map[string]interface{})
		for k, v := range backupRepository.RepositoryParameters {
			params[k] = v
		}
		return GetS3PETMFromParamsMap(params, logger)
	default:
		errMsg := fmt.Sprintf("Unsupported backuprepository driver type: %s. Only support %s.", backupRepository.RepositoryDriver, S3RepositoryDriver)
		return nil, errors.New(errMsg)
	}
}

/*
 * Get the configuration to access the Supervisor namespace from the GuestCluster.
 * This routine will be called only for guest cluster.
 *
 * The secret to access the Supervisor Cluster will be written in the backup driver
 * namespace. The steps to get the Supervisor cluster config are:
 * 1. Create the backup-driver namespace if it does not exist. This is a fixed namespace
 * where the para virt backup driver secret will be written.
 * 2. Wait for the para virt backup driver secret to be written.
 * 3. Get the supervisor cluster configuration from the cert and token in the secret.
 * TODO: Handle update of the para virt backup driver secret
 */
func GetSupervisorConfig(guestConfig *rest.Config, logger logrus.FieldLogger) (*rest.Config, string, error) {
	var err error
	if guestConfig == nil {
		guestConfig, err = rest.InClusterConfig()
		if err != nil {
			logger.WithError(err).Errorf("Failed to get k8s inClusterConfig")
			return nil, "", errors.Wrap(err, "Could not retrieve in-cluster config")
		}
	}
	clientset, err := kubernetes.NewForConfig(guestConfig)
	if err != nil {
		logger.WithError(err).Error("Failed to get k8s clientset from the given config")
		return nil, "", errors.Wrap(err, "Could not retrieve in-cluster config client")
	}

	// Create Backup driver namespace if it does not exist
	err = checkAndCreateNamespace(guestConfig, BackupDriverNamespace, logger)
	if err != nil {
		logger.WithError(err).Errorf("Failed to create namespace %s", BackupDriverNamespace)
		return nil, "", err
	}

	// Check for the para virt secret. If it does not exist, wait for the secret to be written.
	// We wait indefinitely for the secret to be written. It can happen if the backup driver
	// is installed in the guest cluster without velero being installed in the supervisor cluster.
	secretApis := clientset.CoreV1().Secrets(BackupDriverNamespace)
	secret, err := secretApis.Get(context.TODO(), PvSecretName, metav1.GetOptions{})
	if err == nil {
		logger.Infof("Retrieved k8s secret %s in namespace %s", PvSecretName, BackupDriverNamespace)
	} else {
		logger.Infof("Waiting for secret %s to be created in namespace %s", PvSecretName, BackupDriverNamespace)
		ctx := context.Background()
		secret, err = waitForPvSecret(ctx, clientset, BackupDriverNamespace, logger)
		// Failed to get para virt secret
		if err != nil {
			logger.WithError(err).Errorf("Failed to get k8s secret %s in namespace %s", PvSecretName, BackupDriverNamespace)
			return nil, "", err
		}
	}

	// Get data from the secret
	svcNamespace := string(secret.Data["namespace"])
	svcCrt := secret.Data["ca.crt"]
	svcToken := string(secret.Data["token"])

	// Create supervisor config from the data in the secret
	tlsClientConfig := rest.TLSClientConfig{}
	tlsClientConfig.CAData = svcCrt

	return &rest.Config{
		// TODO: switch to using cluster DNS.
		Host:            "https://" + net.JoinHostPort(PvApiEndpoint, PvPort),
		TLSClientConfig: tlsClientConfig,
		BearerToken:     svcToken,
	}, svcNamespace, nil
}

/*
 * Get Supervisor parameters present as annotations in the supervisor namespace for the guest.
 * We do not return all the annotations, but only annotations required but guest cluster plugin.
 */
func GetSupervisorParameters(config *rest.Config, ns string, logger logrus.FieldLogger) (map[string]string, error) {
	params := make(map[string]string)
	var err error
	if config == nil || ns == "" {
		config, ns, err = GetSupervisorConfig(nil, logger)
		if err != nil {
			logger.WithError(err).Error("Could not get supervisor config")
			return params, err
		}
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		logger.WithError(err).Error("Failed to get k8s clientset from the given config")
		return params, err
	}

	nsapi, err := clientset.CoreV1().Namespaces().Get(context.TODO(), ns, metav1.GetOptions{})
	if err != nil {
		logger.WithError(err).Errorf("Could not get namespace object for supervisor namespace %s", ns)
		return params, err
	}

	nsAnnotations := nsapi.ObjectMeta.Annotations

	// Resource pool
	if resPool, ok := nsAnnotations["vmware-system-resource-pool"]; !ok {
		logrus.Warnf("%s information is not present in supervisor namespace %s annotations", SupervisorResourcePoolKey, ns)
	} else {
		params[SupervisorResourcePoolKey] = resPool
	}

	// vCenter UUID and cluster ID
	if svcClusterInfo, ok := nsAnnotations["ncp/extpoolid"]; !ok {
		logrus.Warnf("%s and %s information is not present in supervisor namespace %s annotations", VCuuidKey, SupervisorClusterIdKey, ns)
	} else {
		// Format: <cluster ID>:<vCenter UUID>-ippool-<ip pool range>
		svcClusterParts := strings.Split(svcClusterInfo, ":")
		if len(svcClusterParts) < 2 {
			logrus.Warnf("Invalid ncp/extpoolid %s in supervisor namespace %s annotations", svcClusterInfo, ns)
		} else {
			params[SupervisorClusterIdKey] = svcClusterParts[0]
			vcIdParts := strings.Split(svcClusterParts[1], "-ippool")
			params[VCuuidKey] = vcIdParts[0]
		}
	}
	return params, nil
}

func GetBackupRepositoryFromBackupRepositoryName(backupRepositoryName string) (*backupdriverapi.BackupRepository, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, errors.Wrap(err, "Failed to get k8s inClusterConfig")
	}
	pluginClient, err := plugin_clientset.NewForConfig(config)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to get k8s clientset from the given config: %v ", config)
		return nil, errors.Wrapf(err, errMsg)
	}
	backupRepositoryCR, err := pluginClient.BackupdriverV1().BackupRepositories().Get(context.TODO(), backupRepositoryName, metav1.GetOptions{})
	if err != nil {
		errMsg := fmt.Sprintf("Error while retrieving the backup repository CR %v", backupRepositoryName)
		return nil, errors.Wrapf(err, errMsg)
	}
	return backupRepositoryCR, nil
}

// Retrieve image repository from image name here. We expect the following format
// for image name: <repo-level1>/<repo-level2>/.../<plugin-bin-name>:<tag>. plugin-bin-name
// and tag should not include '/'.
func GetRepo(image string) string {
	if image == "" {
		return ""
	}
	lastIndex := strings.LastIndex(image, "/")
	if lastIndex < 0 {
		return ""
	}
	return image[:lastIndex]
}

/*
 * Create the namespace if it does not already exist.
 */
func checkAndCreateNamespace(config *rest.Config, ns string, logger logrus.FieldLogger) error {
	var err error
	if config == nil {
		config, err = rest.InClusterConfig()
		if err != nil {
			logger.WithError(err).Errorf("Failed to get k8s inClusterConfig")
			return errors.Wrap(err, "Could not retrieve in-cluster config")
		}
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		logger.WithError(err).Error("Failed to get k8s clientset from the given config")
		return err
	}

	// Check if namespace already exists. Return success if namespace already exists.
	_, err = clientset.CoreV1().Namespaces().Get(context.TODO(), ns, metav1.GetOptions{})
	if err == nil {
		logger.Infof("Namespace %s already exists", ns)
		return nil
	}

	// Namespace does not exist. Create it.
	nsSpec := &k8sv1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}
	_, err = clientset.CoreV1().Namespaces().Create(context.TODO(), nsSpec, metav1.CreateOptions{})
	if err != nil {
		logger.WithError(err).Errorf("Could not create namespace %s", ns)
		return err
	}

	logger.Infof("Created namespace %s", ns)
	return nil
}

type waitResult struct {
	item interface{}
	err  error
}

/*
 * Wait for the the para virtual secret to be written in backup driver namespace.
 * The secret will be written by velero-app-operator in the supervisor cluster.
 */
func waitForPvSecret(ctx context.Context, clientSet *kubernetes.Clientset, namespace string, logger logrus.FieldLogger) (*k8sv1.Secret, error) {
	results := make(chan waitResult)
	watchlist := cache.NewListWatchFromClient(clientSet.CoreV1().RESTClient(), "Secrets", namespace,
		fields.OneTermEqualSelector("metadata.name", PvSecretName))
	_, controller := cache.NewInformer(
		watchlist,
		&k8sv1.Secret{},
		time.Second*0,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				secret := obj.(*k8sv1.Secret)
				logger.Infof("secret created: %s", secret.Name)
				results <- waitResult{
					item: secret,
					err:  nil,
				}
			},
			DeleteFunc: func(obj interface{}) {
				logger.Infof("secret deleted: %s", PvSecretName)
				results <- waitResult{
					item: nil,
					err:  errors.New("Not implemented"),
				}
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				logger.Infof("secret updated: %s", PvSecretName)
				results <- waitResult{
					item: nil,
					err:  errors.New("Not implemented"),
				}
			},
		},
	)
	stop := make(chan struct{})
	defer close(stop)

	go controller.Run(stop)
	select {
	case <-ctx.Done():
		stop <- struct{}{}
		return nil, ctx.Err()
	case result := <-results:
		return result.item.(*k8sv1.Secret), result.err
	}
}
