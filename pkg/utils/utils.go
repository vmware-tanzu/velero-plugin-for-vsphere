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
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/constants"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned/typed/backupdriver/v1alpha1"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/plugin/util"
	"io/ioutil"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
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
	pluginv1api "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/datamover/v1alpha1"
	pluginv1client "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned/typed/datamover/v1alpha1"
	plugin_clientset "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
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
	if clusterFlavor == constants.TkgGuest || clusterFlavor == constants.Unknown {
		logger.Errorf("RetrieveVcConfigSecret: Cannot retrieve VC secret in cluster flavor %s", clusterFlavor)
		return errors.New("RetrieveVcConfigSecret: Cannot retrieve VC secret")
	} else if clusterFlavor == constants.Supervisor {
		ns = constants.VCSecretNsSupervisor
		secretData = constants.VCSecretDataSupervisor
	} else {
		ns = constants.VCSecretNs
		secretData = constants.VCSecretData
	}

	secretApis := clientset.CoreV1().Secrets(ns)
	vsphere_secrets := []string{constants.VCSecret, constants.VCSecretTKG}
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
		params["port"] = constants.DefaultVCenterPort
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
	secret, err := secretsClient.Get(context.TODO(), constants.CloudCredentialSecretName, metav1.GetOptions{})
	if err != nil {
		logger.Errorf("RetrieveParamsFromBSL: Failed to retrieve the Secret for %s", constants.CloudCredentialSecretName)
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
		repositoryParams[constants.AWS_ACCESS_KEY_ID] = awsPlainCred.AccessKeyID
		repositoryParams[constants.AWS_SECRET_ACCESS_KEY] = awsPlainCred.SecretAccessKey
		logger.Infof("Successfully retrieved AWS credentials for the BackupStorageLocation.")
		//Breaking since its expected to have only one kv pair for the secret data.
		break
	}

	return nil
}

func RetrieveBSLFromBackup(ctx context.Context, backupName string, config *rest.Config, logger logrus.FieldLogger) (string, error) {
	var err error
	var bslName string
	if config == nil {
		config, err = rest.InClusterConfig()
		if err != nil {
			return bslName, errors.Wrap(err, "Could not retrieve in-cluster config")
		}
	}

	veleroClient, err := versioned.NewForConfig(config)
	if err != nil {
		return bslName, err
	}

	veleroNs, exist := os.LookupEnv("VELERO_NAMESPACE")
	if !exist {
		logger.Errorf("RetrieveBSLFromBackup: Failed to lookup the env variable for velero namespace")
		return bslName, err
	}

	backup, err := veleroClient.VeleroV1().Backups(veleroNs).Get(ctx, backupName, metav1.GetOptions{})
	if err != nil {
		logger.Errorf("RetrieveBSLFromBackup: Backup %s not found", backupName)
		return bslName, err
	}
	bslName = backup.Spec.StorageLocation
	logger.Infof("Found Backup Storage Location %s for the Backup %s", bslName, backupName)

	return bslName, nil
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
	if _, ok := params[constants.AWS_ACCESS_KEY_ID]; ok {
		s3AccessKeyId, ok := GetStringFromParamsMap(params, constants.AWS_ACCESS_KEY_ID, logger)
		if !ok {
			return nil, errors.New("Failed to retrieve S3 Access Key.")
		}
		s3SecretAccessKey, ok := GetStringFromParamsMap(params, constants.AWS_SECRET_ACCESS_KEY, logger)
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
		prefix = constants.DefaultS3RepoPrefix
	}
	s3PETM, err := s3repository.NewS3RepositoryProtectedEntityTypeManager(serviceType, *sess, bucket, prefix, logger)
	if err != nil {
		logger.WithError(err).Errorf("Error at creating new S3 PETM from serviceType: %s, region: %s, bucket: %s",
			serviceType, region, bucket)
		return nil, err
	}

	return s3PETM, nil
}

func GetDefaultS3PETM(logger logrus.FieldLogger) (*s3repository.ProtectedEntityTypeManager, error) {
	var s3PETM *s3repository.ProtectedEntityTypeManager
	var params map[string]interface{}
	err := RetrieveVSLFromVeleroBSLs(params, constants.DefaultS3BackupLocation, nil, logger)
	if err != nil {
		logger.WithError(err).Errorf("GetDefaultS3PETM: Could not retrieve velero default backup location.")
		return nil, err
	}
	logger.Infof("GetDefaultS3PETM: Velero Backup Storage Location is retrieved, region=%s, bucket=%s",
		params["region"], params["bucket"])

	s3PETM, err = GetS3PETMFromParamsMap(params, logger)
	if err != nil {
		logger.WithError(err).Errorf("Failed to get s3PETM from params map, region=%s, bucket=%s",
			params["region"], params["bucket"])
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

func IsFeatureEnabled(feature string, defValue bool, logger logrus.FieldLogger) bool {
	ctx := context.Background()
	if feature == "" {
		return defValue
	}
	config, err := rest.InClusterConfig()
	if err != nil {
		logger.Errorf("Failed to retrieve cluster config: %v", err)
		return defValue
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		logger.Errorf("Failed to retrieve client set: %v", err)
		return defValue
	}

	veleroNs, exist := os.LookupEnv("VELERO_NAMESPACE")
	if !exist {
		logger.Errorf("VELERO_NAMESPACE environment variable not found: %v", err)
		return defValue
	}
	featureConfigMap, err := clientset.CoreV1().ConfigMaps(veleroNs).Get(ctx, constants.VSpherePluginFeatureStates, metav1.GetOptions{})
	if err != nil {
		logger.Errorf("Failed to retrieve feature state config map, recreation may be necessary: %v", err)
		return defValue
	}
	featureData := featureConfigMap.Data
	featureState, err := strconv.ParseBool(featureData[feature])
	if err != nil {
		logger.Errorf("Failed to read feature state from config map using default value, error: %v", err)
		featureState = defValue
	}
	logger.Debugf("Feature: %s Status: %v", feature, featureState)
	return featureState
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

// Check the cluster flavor that the plugin is deployed in
func GetClusterFlavor(config *rest.Config) (constants.ClusterFlavor, error) {
	var err error
	if config == nil {
		config, err = rest.InClusterConfig()
		if err != nil {
			return constants.Unknown, err
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return constants.Unknown, err
	}

	// Direct vSphere deployment.
	// Check if vSphere secret is available in appropriate namespace.
	ns := constants.VCSecretNs
	secretApis := clientset.CoreV1().Secrets(ns)
	vsphere_secrets := []string{constants.VCSecret, constants.VCSecretTKG}
	for _, vsphere_secret := range vsphere_secrets {
		_, err := secretApis.Get(context.TODO(), vsphere_secret, metav1.GetOptions{})
		if err == nil {
			return constants.VSphere, nil
		}
	}

	// Check if in supervisor.
	// Check if vSphere secret is available in appropriate namespace.
	ns = constants.VCSecretNsSupervisor
	secretApis = clientset.CoreV1().Secrets(ns)
	_, err = secretApis.Get(context.TODO(), constants.VCSecret, metav1.GetOptions{})
	if err == nil {
		return constants.Supervisor, nil
	}

	// Check if in guest cluster.
	// Check for the supervisor service in the guest cluster.
	serviceApi := clientset.CoreV1().Services("default")
	_, err = serviceApi.Get(context.TODO(), constants.TkgSupervisorService, metav1.GetOptions{})
	if err == nil {
		return constants.TkgGuest, nil
	}

	// Did not match any search criteria. Unknown cluster flavor.
	return constants.Unknown, errors.New("GetClusterFlavor: Failed to identify cluster flavor")
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
	err = checkAndCreateNamespace(guestConfig, constants.BackupDriverNamespace, logger)
	if err != nil {
		logger.WithError(err).Errorf("Failed to create namespace %s", constants.BackupDriverNamespace)
		return nil, "", err
	}

	// Check for the para virt secret. If it does not exist, wait for the secret to be written.
	// We wait indefinitely for the secret to be written. It can happen if the backup driver
	// is installed in the guest cluster without velero being installed in the supervisor cluster.
	secretApis := clientset.CoreV1().Secrets(constants.BackupDriverNamespace)
	secret, err := secretApis.Get(context.TODO(), constants.PvSecretName, metav1.GetOptions{})
	if err == nil {
		logger.Infof("Retrieved k8s secret %s in namespace %s", constants.PvSecretName, constants.BackupDriverNamespace)
	} else {
		logger.Infof("Waiting for secret %s to be created in namespace %s", constants.PvSecretName, constants.BackupDriverNamespace)
		ctx := context.Background()
		secret, err = waitForPvSecret(ctx, clientset, constants.BackupDriverNamespace, logger)
		// Failed to get para virt secret
		if err != nil {
			logger.WithError(err).Errorf("Failed to get k8s secret %s in namespace %s", constants.PvSecretName, constants.BackupDriverNamespace)
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
		Host:            "https://" + net.JoinHostPort(constants.PvApiEndpoint, constants.PvPort),
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
		logrus.Warnf("%s information is not present in supervisor namespace %s annotations", constants.SupervisorResourcePoolKey, ns)
	} else {
		params[constants.SupervisorResourcePoolKey] = resPool
	}

	// vCenter UUID and cluster ID
	if svcClusterInfo, ok := nsAnnotations["ncp/extpoolid"]; !ok {
		logrus.Warnf("%s and %s information is not present in supervisor namespace %s annotations", constants.VCuuidKey, constants.SupervisorClusterIdKey, ns)
	} else {
		// Format: <cluster ID>:<vCenter UUID>-ippool-<ip pool range>
		svcClusterParts := strings.Split(svcClusterInfo, ":")
		if len(svcClusterParts) < 2 {
			logrus.Warnf("Invalid ncp/extpoolid %s in supervisor namespace %s annotations", svcClusterInfo, ns)
		} else {
			params[constants.SupervisorClusterIdKey] = svcClusterParts[0]
			vcIdParts := strings.Split(svcClusterParts[1], "-ippool")
			params[constants.VCuuidKey] = vcIdParts[0]
		}
	}
	return params, nil
}

func GetComponentFromImage(image string, component string) string {
	components := GetComponentsFromImage(image)
	return components[component]
}

func GetComponentsFromImage(image string) map[string]string {
	components := make(map[string]string)

	if image == "" {
		return components
	}

	var taggedContainer string
	lastIndex := strings.LastIndex(image, "/")
	if lastIndex < 0 {
		taggedContainer = image
	} else {
		components[constants.ImageRepositoryComponent] = image[:lastIndex]
		taggedContainer = image[lastIndex+1:]
	}

	parts := strings.SplitN(taggedContainer, ":", 2)
	if len(parts) == 2 {
		components[constants.ImageVersionComponent] = parts[1]
	}
	components[constants.ImageContainerComponent] = parts[0]

	return components
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
		fields.OneTermEqualSelector("metadata.name", constants.PvSecretName))
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
				logger.Infof("secret deleted: %s", constants.PvSecretName)
				results <- waitResult{
					item: nil,
					err:  errors.New("Not implemented"),
				}
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				logger.Infof("secret updated: %s", constants.PvSecretName)
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

/*
 Adds the Velero label to exclude this K8S resource from the backup
*/
func AddVeleroExcludeLabelToObjectMeta(objectMeta *metav1.ObjectMeta) {
	objectMeta.Labels = AppendVeleroExcludeLabels(objectMeta.Labels)
}

func AppendVeleroExcludeLabels(origLabels map[string]string) map[string]string {
	if origLabels == nil {
		origLabels = make(map[string]string)
	}
	origLabels[constants.VeleroExcludeLabel] = "true"
	return origLabels
}

func GetResources() []string {
	desiredResources := make([]string, len(constants.ResourcesToHandle)+len(constants.ResourcesToBlock))
	for resourceToHandle, _ := range constants.ResourcesToHandle {
		desiredResources = append(desiredResources, resourceToHandle)
	}
	for resourceToBlock, _ := range constants.ResourcesToBlock {
		desiredResources = append(desiredResources, resourceToBlock)
	}
	return desiredResources
}

func IsResourceBlocked(resourceName string) bool {
	return constants.ResourcesToBlock[resourceName]
}

func IsResourceBlockedOnRestore(resourceName string) bool {
	return constants.ResourcesToBlockOnRestore[resourceName]
}

func IsObjectBlocked(item runtime.Unstructured) (bool, string, error) {
	// Retrieve the common object metadata and check to see if this is a blocked type
	accessor := meta.NewAccessor()

	selfLink, err := accessor.SelfLink(item)
	if err != nil {
		return false, "", errors.Wrap(err, "Failed to retrieve SelfLink")
	}
	crdName := util.SelfLinkToCRDName(selfLink)
	if crdName == "" {
		return false, "", errors.Errorf("Could not translate selfLink %s to CRD name", selfLink)
	}
	if IsResourceBlocked(crdName) {
		return true, crdName, nil
	}
	return false, crdName, nil
}

func GetBackupdriverClient(config *rest.Config) (v1alpha1.BackupdriverV1alpha1Interface, error) {
	pluginClient, err := plugin_clientset.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return pluginClient.BackupdriverV1alpha1(), nil
}

// Provide a utility function in guest cluster to clean up corresponding supervisor cluster snapshot CR
func DeleteSvcSnapshot(svcSnapshotName string, gcSnapshotName string, gcSnapshotNamespace string, guestConfig *rest.Config, logger logrus.FieldLogger) error {
	if svcSnapshotName == "" {
		errMsg := fmt.Sprintf("No corresponding supervisor cluster snapshot CR name for guest cluster snapshot %s/%s", gcSnapshotNamespace, gcSnapshotName)
		logger.Error(errMsg)
		return errors.New(errMsg)
	}
	svcConfig, svcNamespace, err := GetSupervisorConfig(guestConfig, logger)
	if err != nil {
		logger.WithError(err).Error("Failed to get supervisor config from given guest config")
		return err
	}
	if svcNamespace == "" {
		errMsg := fmt.Sprintf("Failed to retrieve svcNamespace")
		logger.Error(errMsg)
		return errors.New(errMsg)
	}
	backupdriverClient, err := GetBackupdriverClient(svcConfig)
	if err != nil {
		logger.WithError(err).Error("Failed to retrieve plugin client from given svcConfig")
		return err
	}
	if err = backupdriverClient.Snapshots(svcNamespace).Delete(context.TODO(), svcSnapshotName, metav1.DeleteOptions{}); k8serrors.IsNotFound(err) {
		logger.Infof("SvcSnapshot %s/%s is already deleted, no need to process it", svcNamespace, svcSnapshotName)
		return nil
	} else if err != nil {
		logger.WithError(err).Errorf("Failed to delete supervisor cluster snapshot %s/%s", svcNamespace, svcSnapshotName)
		return err
	}
	return nil
}

