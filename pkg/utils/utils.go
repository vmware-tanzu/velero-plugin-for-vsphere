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

	"github.com/hashicorp/go-version"
	vcConfig "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/common/config"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/constants"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned/typed/backupdriver/v1alpha1"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/ivd"
	"gopkg.in/gcfg.v1"
	"k8s.io/client-go/tools/clientcmd"

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/tools/cache"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	jsonpatch "github.com/evanphx/json-patch"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/vmware-tanzu/astrolabe/pkg/astrolabe"
	"github.com/vmware-tanzu/astrolabe/pkg/s3repository"
	datamover_api "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/datamover/v1alpha1"
	plugin_clientset "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned"
	datamover_client "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/clientset/versioned/typed/datamover/v1alpha1"
	velero_api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	velero_clientset "github.com/vmware-tanzu/velero/pkg/generated/clientset/versioned"
	k8sv1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func RetrieveVcConfigSecret(params map[string]interface{}, config *rest.Config, logger logrus.FieldLogger) error {
	var err error // Declare here to avoid shadowing on using in cluster config only
	if config == nil {
		config, err = GetKubeClientConfig()
		if err != nil {
			logger.WithError(err).Errorf("Failed to get k8s inClusterConfig")
			return errors.Wrap(err, "Could not retrieve in-cluster config")
		}
	}
	clientset, err := GetKubeClientSet(config)
	if err != nil {
		logger.WithError(err).Errorf("Failed to get k8s clientset from the given config: %v", config)
		return err
	}
	veleroNs, exist := GetVeleroNamespace()
	if !exist {
		logger.Errorf("RetrieveVcConfigSecret: Failed to lookup the env variable for velero namespace, using " +
			"default velero")
		veleroNs = constants.DefaultVeleroNamespace
	}
	var ns, name string
	var vSphereSecrets []string
	// Get the cluster flavor
	clusterFlavor, err := retrieveClusterFlavor(clientset, veleroNs)
	if clusterFlavor == constants.Unknown {
		logger.Errorf("RetrieveVcConfigSecret: Cannot retrieve VC secret in cluster flavor %s", clusterFlavor)
		return errors.New("RetrieveVcConfigSecret: Cannot retrieve VC secret as the cluster flavor is UNKNOWN")
	} else if clusterFlavor == constants.TkgGuest {
		logger.Errorf("RetrieveVcConfigSecret: Retrieving VC secret in cluster flavor %s is not supported", clusterFlavor)
		return errors.New("RetrieveVcConfigSecret: Retrieving VC Credentials Secret is not supported in Guest Cluster.")
	} else if clusterFlavor == constants.Supervisor {
		ns = constants.VCSecretNsSupervisor
		vSphereSecrets = append(vSphereSecrets, constants.VCSecret, constants.VCSecretTKG)
	} else { // constants.VSphere
		if IsFeatureEnabled(clientset, constants.DecoupleVSphereCSIDriverFlag, true, logger) {
			// Retrieve the vc credentials secret name and namespace from velero-vsphere-plugin-config
			ns, name = GetSecretNamespaceAndName(clientset, veleroNs, constants.VeleroVSpherePluginConfig)
			vSphereSecrets = append(vSphereSecrets, name)
			logger.Debugf("RetrieveVcConfigSecret: Namespace: %s Name: %s", ns, name)
		} else {
			// check the current CSI version for vanilla setups.
			// If >=2.3, then the secret is in vmware-system-csi
			// If <2.3, then the secret is in kube-system
			csiInstalledVersion, err := GetCSIInstalledVersion(clientset)
			if err != nil {
				logger.WithError(err).Errorf("Unable to find installed CSI drivers while retrieving VC configuration Secret")
				return err
			}
			if CompareVersion(csiInstalledVersion, constants.Csi2_3_0_Version) >= 0 {
				ns = constants.VCSystemCSINs
			} else {
				ns = constants.VCSecretNs
			}
			vSphereSecrets = append(vSphereSecrets, constants.VCSecret, constants.VCSecretTKG)
		}
	}

	secretApis := clientset.CoreV1().Secrets(ns)
	var secret *k8sv1.Secret
	for _, vSphereSecret := range vSphereSecrets {
		secret, err = secretApis.Get(context.TODO(), vSphereSecret, metav1.GetOptions{})
		if err == nil {
			logger.Debugf("Retrieved k8s secret, %s", vSphereSecret)
			break
		}
		logger.WithError(err).Infof("Skipping k8s secret %s as it does not exist", vSphereSecret)
	}

	// No valid secret found.
	if err != nil {
		logger.WithError(err).Errorf("failed to retrieve vSphere credentials Secret, %s", vSphereSecrets)
		return err
	}
	// No kv pairs in the secret.
	if len(secret.Data) == 0 {
		errMsg := fmt.Sprintf("failed to get any data in k8s secret, %s", vSphereSecrets)
		logger.Errorf(errMsg)
		return errors.New(errMsg)
	}

	err = ParseConfig(secret, params, logger)
	if err != nil {
		logger.WithError(err).Error("Failed to parse vSphere secret data")
		return err
	}

	return nil
}

func ParseConfig(secret *k8sv1.Secret, params map[string]interface{}, logger logrus.FieldLogger) error {
	// Setup config
	var conf vcConfig.Config
	// Read secret data
	for _, value := range secret.Data {
		confStr := string(value) // Convert the secret data to a string
		err := gcfg.FatalOnly(gcfg.ReadStringInto(&conf, confStr))
		if err != nil {
			logger.WithError(err).Error("Failed to parse vSphere secret data")
			return err
		}
		logger.Debugf("Successfully parsed vCenter configuration from secret %s", secret.Name)
		break
	}

	// Use config data from struct to populate params map passed by RetrieveVcConfigSecret callers
	params["cluster-id"] = conf.Global.ClusterID

	for ip, vcConfig := range conf.VirtualCenter {
		params["VirtualCenter"] = ip
		params["user"] = vcConfig.User
		params["password"] = vcConfig.Password
		if vcConfig.VCenterPort == "" {
			vcConfig.VCenterPort = constants.DefaultVCenterPort
		}
		params["port"] = vcConfig.VCenterPort
	}

	return nil
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

	veleroNs, exist := GetVeleroNamespace()
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
		config, err = GetKubeClientConfig()
		if err != nil {
			return bslName, errors.Wrap(err, "Could not retrieve in-cluster config")
		}
	}

	veleroClient, err := velero_clientset.NewForConfig(config)
	if err != nil {
		return bslName, err
	}

	veleroNs, exist := GetVeleroNamespace()
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
	var err error // Declare here to avoid shadowing on using in cluster config only
	if config == nil {
		config, err = GetKubeClientConfig()
		if err != nil {
			return errors.Wrap(err, "Could not retrieve in-cluster config")
		}
	}

	veleroClient, err := velero_clientset.NewForConfig(config)
	if err != nil {
		return err
	}

	veleroNs, exist := GetVeleroNamespace()
	if !exist {
		logger.Errorf("RetrieveVSLFromVeleroBSLs: Failed to lookup the env variable for velero namespace")
		return err
	}

	var backupStorageLocation *velero_api.BackupStorageLocation
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

	if backupStorageLocation.Spec.ObjectStorage.CACert != nil {
		params["caCert"] = string(backupStorageLocation.Spec.ObjectStorage.CACert)
	}

	return nil
}

func GetIVDPETMFromParamsMap(params map[string]interface{}, logger logrus.FieldLogger) (astrolabe.ProtectedEntityTypeManager, error) {
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

	ivdPETM, err := ivd.NewIVDProtectedEntityTypeManager(params, s3Config, logger)
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

	sessionOption, err := GetS3SessionOptionsFromParamsMap(params, logger)
	if err != nil {
		logger.WithError(err).Error("Failed to get s3 session option from params.")
		return nil, err
	}
	sess := session.Must(session.NewSessionWithOptions(sessionOption))

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

func GetS3SessionOptionsFromParamsMap(params map[string]interface{}, logger logrus.FieldLogger) (session.Options, error) {
	region, _ := GetStringFromParamsMap(params, "region", logger)
	// If the credentials are explicitly provided in params, use it.
	// else let aws API pick the default credential provider.
	sessionOptions := session.Options{Config: aws.Config{
		Region: aws.String(region),
	}}
	var credential *credentials.Credentials
	if _, ok := params[constants.AWS_ACCESS_KEY_ID]; ok {
		s3AccessKeyId, ok := GetStringFromParamsMap(params, constants.AWS_ACCESS_KEY_ID, logger)
		if !ok {
			return session.Options{}, errors.New("Failed to retrieve S3 Access Key.")
		}
		s3SecretAccessKey, ok := GetStringFromParamsMap(params, constants.AWS_SECRET_ACCESS_KEY, logger)
		if !ok {
			return session.Options{}, errors.New("Failed to retrieve S3 Secret Access Key.")
		}
		logger.Infof("Using explicitly found credentials for S3 repository access.")
		credential = credentials.NewStaticCredentials(s3AccessKeyId, s3SecretAccessKey, "")
		sessionOptions.Config.Credentials = credential
	}
	caCert, ok := GetStringFromParamsMap(params, "caCert", logger)
	if ok && len(caCert) > 0 {
		sessionOptions.CustomCABundle = strings.NewReader(caCert)
	}
	return sessionOptions, nil
}

func GetDefaultS3PETM(logger logrus.FieldLogger) (*s3repository.ProtectedEntityTypeManager, error) {
	var s3PETM *s3repository.ProtectedEntityTypeManager
	params := make(map[string]interface{})
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
		logger.Infof("No such key %s in params map", key)
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

func IsFeatureEnabled(clientset kubernetes.Interface, feature string, defValue bool, logger logrus.FieldLogger) bool {
	ctx := context.Background()
	var err error
	if feature == "" {
		return defValue
	}
	veleroNs, exist := GetVeleroNamespace()
	if !exist {
		logger.Errorf("VELERO_NAMESPACE environment variable not found, using default: %s, err: %v", constants.DefaultVeleroNamespace, err)
		veleroNs = constants.DefaultVeleroNamespace
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
	config, err := GetKubeClientConfig()
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

func PatchUpload(req *datamover_api.Upload, mutate func(*datamover_api.Upload), uploadClient datamover_client.UploadInterface, logger logrus.FieldLogger) (*datamover_api.Upload, error) {

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

func RetrieveClusterFlavor(config *rest.Config, veleroNs string) (constants.ClusterFlavor, error) {
	var err error
	if config == nil {
		config, err = GetKubeClientConfig()
		if err != nil {
			return constants.Unknown, err
		}
	}

	clientset, err := GetKubeClientSet(config)
	if err != nil {
		return constants.Unknown, err
	}

	return retrieveClusterFlavor(clientset, veleroNs)
}

func GetClusterFlavor(config *rest.Config) (constants.ClusterFlavor, error) {
	var err error
	if config == nil {
		config, err = GetKubeClientConfig()
		if err != nil {
			return constants.Unknown, err
		}
	}

	clientset, err := GetKubeClientSet(config)
	if err != nil {
		return constants.Unknown, err
	}

	veleroNs, exist := GetVeleroNamespace()
	if !exist {
		fmt.Printf("The VELERO_NAMESPACE environment variable is empty, assuming velero as namespace\n")
		veleroNs = constants.DefaultVeleroNamespace
	}

	return retrieveClusterFlavor(clientset, veleroNs)
}

func retrieveClusterFlavor(clientset kubernetes.Interface, veleroNs string) (constants.ClusterFlavor, error) {
	var err error
	// Check for velero-vsphere-plugin-config in the velero installation namespace
	clusterFlavor, err := GetClusterTypeFromConfig(clientset, veleroNs, constants.VeleroVSpherePluginConfig)
	if err != nil {
		fmt.Printf("Error received while retrieving cluster flavor from config, err: %+v\n", err)
		clusterFlavor = constants.Unknown
	}
	if clusterFlavor == constants.Unknown {
		fmt.Printf("Falling back to retrieving cluster flavor from vSphere CSI Driver Deployment\n")
		clusterFlavor, err = GetCSIClusterType(clientset)
		if err != nil {
			fmt.Printf("Received error while retrieving cluster flavor from vSphere CSI Driver, err: %+v\n", err)
			clusterFlavor = constants.Unknown
		}
	}
	if clusterFlavor == constants.Unknown {
		fmt.Printf("Failed to retrieve cluster flavor from Velero Vsphere Plugin Config or vSphere CSI Driver\n")
		fmt.Printf("Defaulting the cluster flavor to VANILLA\n")
		clusterFlavor = constants.VSphere
	}
	return clusterFlavor, nil
}

func GetClusterTypeFromConfig(kubeClient kubernetes.Interface, veleroNs string, configName string) (constants.ClusterFlavor, error) {
	veleroPuginConfigMap, err := kubeClient.CoreV1().ConfigMaps(veleroNs).Get(context.TODO(), configName, metav1.GetOptions{})
	if err != nil {
		fmt.Printf("failed to retrieve %s configuration, err: %+v.\n", configName, err)
		return constants.Unknown, err
	}
	configMap := veleroPuginConfigMap.Data
	if flavor, ok := configMap[constants.ConfigClusterFlavorKey]; ok {
		clusterFlavor := ConvertConfigClusterFlavor(flavor)
		return clusterFlavor, nil
	}
	return constants.Unknown, nil
}

func ConvertConfigClusterFlavor(flavor string) constants.ClusterFlavor {
	switch flavor {
	case "VANILLA":
		return constants.VSphere
	case "GUEST":
		return constants.TkgGuest
	case "SUPERVISOR":
		return constants.Supervisor
	default:
		return constants.Unknown
	}
}

func GetSecretNamespaceAndName(kubeClient kubernetes.Interface, veleroNs string, configName string) (string, string) {
	var namespace, name string
	veleroPuginConfigMap, err := kubeClient.CoreV1().ConfigMaps(veleroNs).Get(context.TODO(), configName, metav1.GetOptions{})
	if err != nil {
		fmt.Printf("Failed to retrieve %s configuration, using defaults, err: %+v.\n", configName, err)
		return constants.DefaultSecretNamespace, constants.DefaultSecretName
	}
	configMap := veleroPuginConfigMap.Data
	if _, ok := configMap[constants.VSphereSecretNamespaceKey]; !ok {
		namespace = constants.DefaultSecretNamespace
	} else {
		namespace = configMap[constants.VSphereSecretNamespaceKey]
	}
	if _, ok := configMap[constants.VSphereSecretNameKey]; !ok {
		name = constants.DefaultSecretName
	} else {
		name = configMap[constants.VSphereSecretNameKey]
	}
	return namespace, name
}

func GetUploadCRRetryMaximumFromConfig(kubeClient kubernetes.Interface, veleroNs string, configName string, logger logrus.FieldLogger) int {
	veleroPuginConfigMap, err := kubeClient.CoreV1().ConfigMaps(veleroNs).Get(context.TODO(), configName, metav1.GetOptions{})
	if err != nil {
		logger.Errorf("failed to retrieve %s configuration, err: %+v.\n", configName, err)
		return constants.DefaultUploadCRRetryMaximum
	}
	configMap := veleroPuginConfigMap.Data
	if maxRetryCountStr, ok := configMap[constants.UploadCRRetryMaximumKey]; ok {
		maxRetryCount, err := strconv.Atoi(maxRetryCountStr)
		if err != nil {
			logger.Errorf("UploadCRRretryMaximum %s is invalid, err: %+v.\n", maxRetryCountStr, err)
			maxRetryCount = constants.DefaultUploadCRRetryMaximum
		}
		return maxRetryCount
	}
	return constants.DefaultUploadCRRetryMaximum
}

func GetUploadCRRetryMaximum(config *rest.Config, logger logrus.FieldLogger) int {
	var err error
	if config == nil {
		config, err = GetKubeClientConfig()
		if err != nil {
			logger.Errorf("GetKubeClientConfig failed with err: %+v, UploadCRRetryMaximum is set to default: %d", err, constants.DefaultUploadCRRetryMaximum)
			return constants.DefaultUploadCRRetryMaximum
		}
	}

	clientset, err := GetKubeClientSet(config)
	if err != nil {
		logger.Errorf("GetKubeClienSet failed with err: %+v, UploadCRRetryMaximum is set to default: %d", err, constants.DefaultUploadCRRetryMaximum)
		return constants.DefaultUploadCRRetryMaximum
	}

	veleroNs, exist := GetVeleroNamespace()
	if !exist {
		logger.Errorf("The VELERO_NAMESPACE environment variable is empty, assuming velero as namespace\n")
		veleroNs = constants.DefaultVeleroNamespace
	}

	// Check for velero-vsphere-plugin-config in the velero installation namespace
	maxRetryCnt := GetUploadCRRetryMaximumFromConfig(clientset, veleroNs, constants.VeleroVSpherePluginConfig, logger)
	return maxRetryCnt
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
		guestConfig, err = GetKubeClientConfig()
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
		config, err = GetKubeClientConfig()
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

func GetVcConfigSecretFilterFunc(logger logrus.FieldLogger) func(obj interface{}) bool {
	config, err := GetKubeClientConfig()
	if err != nil {
		logger.WithError(err).Errorf("Failed to get k8s inClusterConfig")
		return nil
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		logger.WithError(err).Errorf("Failed to get k8s clientset from the given config")
		return nil
	}

	veleroNs, exist := GetVeleroNamespace()
	if !exist {
		logger.Errorf("RetrieveVcConfigSecret: Failed to lookup the env variable for velero namespace, using "+
			"default %s", constants.DefaultVeleroNamespace)
		veleroNs = constants.DefaultVeleroNamespace
	}

	var ns, name string

	// Get the cluster flavor
	clusterFlavor, err := retrieveClusterFlavor(clientset, veleroNs)
	if clusterFlavor == constants.Supervisor {
		ns = constants.VCSecretNsSupervisor
		name = constants.VCSecret
	} else if clusterFlavor == constants.VSphere {
		if IsFeatureEnabled(clientset, constants.DecoupleVSphereCSIDriverFlag, true, logger) {
			// Retrieve the vc credentials secret name and namespace from velero-vsphere-plugin-config
			ns, name = GetSecretNamespaceAndName(clientset, veleroNs, constants.VeleroVSpherePluginConfig)
			logger.Infof("RetrieveVcConfigSecret: Namespace: %s Name: %s", ns, name)
		} else {
			// check the current CSI version for vanilla setups.
			// If >=2.3, then the secret is in vmware-system-csi namespace
			// If <2.3, then the secret is in kube-system namespace
			csiInstalledVersion, err := GetCSIInstalledVersion(clientset)
			if err != nil {
				logger.WithError(err).Errorf("unable to find installed CSI drivers during VC configuration Secret watch filtering.")
			}
			logger.Infof("CSI version : %s for VC configuration Secret watch.", csiInstalledVersion)
			if CompareVersion(csiInstalledVersion, constants.Csi2_3_0_Version) >= 0 {
				ns = constants.VCSystemCSINs
			} else {
				ns = constants.VCSecretNs
			}
			name = constants.VCSecret
		}
	}
	logger.Infof("VC Configuration Secret: Namespace: %s Name: %s", ns, name)
	return func(obj interface{}) bool {
		switch obj.(type) {
		case *k8sv1.Secret:
			incomingSecret := obj.(*k8sv1.Secret)
			return incomingSecret.Namespace == ns &&
				incomingSecret.Name == name
		default:
			logger.Debugf("Unrecognized object type found during filtering, ignoring")
		}
		return false
	}
}

type ClientConfigNotFoundError struct {
	errMsg string
}

func (this ClientConfigNotFoundError) Error() string {
	return this.errMsg
}

func NewClientConfigNotFoundError(errMsg string) ClientConfigNotFoundError {
	err := ClientConfigNotFoundError{
		errMsg: errMsg,
	}
	return err
}

func GetKubeClientConfig() (*rest.Config, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	// if you want to change the loading rules (which files in which order), you can do so here

	configOverrides := &clientcmd.ConfigOverrides{}
	// if you want to change override values or bind them to flags, there are methods to help you

	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	clientConfig, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, errors.Wrap(err, "Error finding Kubernetes API server config in $KUBECONFIG, or in-cluster configuration")
	}

	return clientConfig, nil
}

func CreateKubeClientSet() (*kubernetes.Clientset, error) {
	clientConfig, err := GetKubeClientConfig()
	if err != nil {
		return nil, NewClientConfigNotFoundError(fmt.Sprintf("Could not get client config with err: %v", err))
	}

	clientset, err := kubernetes.NewForConfig(clientConfig)
	if err != nil {
		return nil, errors.Wrap(err, "Could not create kubernetes clientset")
	}
	return clientset, err
}

func CreatePluginClientSet() (*plugin_clientset.Clientset, error) {
	clientConfig, err := GetKubeClientConfig()
	if err != nil {
		return nil, NewClientConfigNotFoundError(fmt.Sprintf("Could not get client config with err: %v", err))
	}

	pluginClientSet, err := plugin_clientset.NewForConfig(clientConfig)
	if err != nil {
		return nil, errors.Wrapf(err, "Could not create plugin clientset with the given config: %v", clientConfig)
	}
	return pluginClientSet, err
}

func GetCSIInstalledVersion(kubeClient kubernetes.Interface) (string, error) {
	csiVersion := ""
	// For CSI >=2.3.0, the CSI Deployment is in vmware-system-csi namespace.
	csiDeployment, err := kubeClient.AppsV1().Deployments(constants.VSphereCSIControllerNamespace).Get(context.TODO(), constants.VSphereCSIController, metav1.GetOptions{})
	if err == nil {
		if csiVersion, err = GetCSIVersionFromImage(csiDeployment.Spec.Template.Spec.Containers); err != nil {
			// Image not found.
			return "", err
		}
		if csiVersion == constants.CsiDevVersion {
			// Using a unreleased csi version, defaulting to 2.3.0 since vSphere csi driver 2.3.0
			// is always installed in vmware-system-csi namespace.
			return constants.Csi2_3_0_Version, nil
		}
		return csiVersion, nil
	}

	// Search in kube-system next.
	csiDeployment, err = kubeClient.AppsV1().Deployments(constants.KubeSystemNamespace).Get(context.TODO(), constants.VSphereCSIController, metav1.GetOptions{})
	if err == nil {
		if csiVersion, err = GetCSIVersionFromImage(csiDeployment.Spec.Template.Spec.Containers); err != nil {
			// Image not found.
			return "", err
		}
		if csiVersion == constants.CsiDevVersion {
			// Using unreleased version, defaulting to 2.1.0 since vSphere CSI driver is installed as a
			// Deployment on `kube-system` for versions greater than v1.0.3 and lesser than v2.3.0
			return constants.Csi2_0_0_Version, nil
		}
		return csiVersion, nil
	}
	// CSI driver is deployed as StatefulSet for v1.0.2 and v1.0.3.
	csiStatefulset, err := kubeClient.AppsV1().StatefulSets(constants.KubeSystemNamespace).Get(context.TODO(), constants.VSphereCSIController, metav1.GetOptions{})
	if err == nil {
		if csiVersion, err = GetCSIVersionFromImage(csiStatefulset.Spec.Template.Spec.Containers); err != nil {
			// Image not found.
			return "", err
		}
		if csiVersion == constants.CsiDevVersion {
			// Using unreleased version, defaulting to 1.0.2 since vSphere CSI driver is installed as a
			// StatefulSet on `kube-system` namespace for versions greater than v1.0.2 and lesser than v2.0.0
			return constants.CsiMinVersion, nil
		}
		return csiVersion, nil
	}

	// vSphere CSI controller not installed.
	return csiVersion, errors.Errorf("vSphere CSI controller, %s, is required by velero-plugin-for-vsphere. Please make sure the vSphere CSI controller is installed in the cluster", constants.VSphereCSIController)
}

func GetCSIClusterType(kubeClient kubernetes.Interface) (constants.ClusterFlavor, error) {
	csiClusterType := constants.Unknown
	// For CSI >=2.3.0, the CSI Deployment is in vmware-system-csi namespace.
	csiDeployment, err := kubeClient.AppsV1().Deployments(constants.VSphereCSIControllerNamespace).Get(context.TODO(), constants.VSphereCSIController, metav1.GetOptions{})
	if err == nil {
		if csiClusterType, err = GetCSIClusterTypeFromEnv(csiDeployment.Spec.Template.Spec.Containers); err != nil {
			// Image not found.
			return constants.Unknown, err
		}
		return csiClusterType, nil
	}
	// Search in kube-system next.
	csiDeployment, err = kubeClient.AppsV1().Deployments(constants.KubeSystemNamespace).Get(context.TODO(), constants.VSphereCSIController, metav1.GetOptions{})
	if err == nil {
		if csiClusterType, err = GetCSIClusterTypeFromEnv(csiDeployment.Spec.Template.Spec.Containers); err != nil {
			// Image not found.
			return constants.Unknown, err
		}
		return csiClusterType, nil
	}
	// CSI driver is deployed as StatefulSet for v1.0.2 and v1.0.3.
	csiStatefulset, err := kubeClient.AppsV1().StatefulSets(constants.KubeSystemNamespace).Get(context.TODO(), constants.VSphereCSIController, metav1.GetOptions{})
	if err == nil {
		if csiClusterType, err = GetCSIClusterTypeFromEnv(csiStatefulset.Spec.Template.Spec.Containers); err != nil {
			// Image not found.
			return constants.Unknown, err
		}
		return csiClusterType, nil
	}
	// vSphere CSI controller not installed.
	return csiClusterType, errors.Errorf("vSphere CSI controller, %s, is required by velero-plugin-for-vsphere. Please make sure the vSphere CSI controller is installed in the cluster", constants.VSphereCSIController)
}

func GetCSIVersionFromImage(containers []k8sv1.Container) (string, error) {
	var csiDriverVersion string
	csiDriverVersion = GetVersionFromImage(containers, "cloud-provider-vsphere/csi/release/driver")
	if csiDriverVersion == "" {
		csiDriverVersion = GetVersionFromImage(containers, "cloud-provider-vsphere/csi/ci/driver")
		// The version received from cloud-provider-vsphere/csi/ci/driver is not nil.
		// This is used to support latest images on public repository.
		if csiDriverVersion != "" {
			// Developer image
			csiDriverVersion = constants.CsiDevVersion
		}
	}
	if csiDriverVersion == "" {
		// vSphere driver found but the images are invalid.
		return "", errors.New("Expected CSI driver images not found")
	}
	return csiDriverVersion, nil
}

func GetCSIClusterTypeFromEnv(containers []k8sv1.Container) (constants.ClusterFlavor, error) {
	for _, container := range containers {
		if container.Name == constants.VSphereCSIController {
			// Iterate through the env variables and check if "CLUSTER_FLAVOR" env variable is defined.
			for _, envVar := range container.Env {
				if envVar.Name == "CLUSTER_FLAVOR" {
					if envVar.Value == "WORKLOAD" {
						return constants.Supervisor, nil
					} else if envVar.Value == "GUEST_CLUSTER" {
						return constants.TkgGuest, nil
					} else {
						// Should not happen, if "CLUSTER_FLAVOR" is defined, the value should be guest or supervisor.
						return constants.Unknown, nil
					}
				}
			}
			// For Vanilla deployment the "CLUSTER_FLAVOR" environment variable is not defined.
			return constants.VSphere, nil
		}
	}
	return constants.Unknown, errors.New("Expected CSI driver container images not found while inferring cluster type.")
}

// Return version in the format: vX.Y.Z
func GetVersionFromImage(containers []k8sv1.Container, imageName string) string {
	var tag = ""
	for _, container := range containers {
		if strings.Contains(container.Image, imageName) {
			tag = GetComponentFromImage(container.Image, constants.ImageVersionComponent)
			break
		}
	}
	if tag == "" {
		return ""
	}
	if strings.Contains(tag, "-") {
		imgVersion := strings.Split(tag, "-")[0]
		return imgVersion
	} else {
		return tag
	}
}

// If currentVersion < minVersion, return -1
// If currentVersion == minVersion, return 0
// If currentVersion > minVersion, return 1
// Assume input versions are both valid
func CompareVersion(currentVersion string, minVersion string) int {
	current, _ := version.NewVersion(currentVersion)
	minimum, _ := version.NewVersion(minVersion)

	if current == nil || minimum == nil {
		return -1
	}
	return current.Compare(minimum)
}

func RetrieveVddkLogLevel(params map[string]interface{}, logger logrus.FieldLogger) error {
	var err error // Declare here to avoid shadowing on using in cluster config only
	kubeClient, err := CreateKubeClientSet()
	if err != nil {
		logger.WithError(err).Error("Failed to create kube clientset")
		return err
	}
	veleroNs, exist := GetVeleroNamespace()
	if !exist {
		errMsg := "Failed to lookup the ENV variable for velero namespace"
		logger.Error(errMsg)
		return errors.New(errMsg)
	}
	opts := metav1.ListOptions{
		// velero.io/vddk-config: vix-disk-lib
		LabelSelector: fmt.Sprintf("%s=%s", constants.VddkConfigLabelKey, constants.VixDiskLib),
	}
	vddkConfigMaps, err := kubeClient.CoreV1().ConfigMaps(veleroNs).List(context.TODO(), opts)
	if err != nil {
		logger.WithError(err).Error("Failed to retrieve config map lists for vddk config")
		return err
	}
	if len(vddkConfigMaps.Items) == 0 {
		logger.Info("No customized config map for vddk exists")
		return nil
	}
	if len(vddkConfigMaps.Items) > 1 {
		var items []string
		for _, item := range vddkConfigMaps.Items {
			items = append(items, item.Name)
		}
		return errors.Errorf("found more than one ConfigMap matching label selector %q: %v", opts.LabelSelector, items)
	}
	if _, ok := params[constants.VddkConfig]; !ok {
		params[constants.VddkConfig] = make(map[string]string)
	}
	for k, v := range vddkConfigMaps.Items[0].Data {
		params[constants.VddkConfig].(map[string]string)[k] = v
	}
	return nil
}

func GetKubeClientSet(config *rest.Config) (kubernetes.Interface, error) {
	return kubernetes.NewForConfig(config)
}

func GetVeleroNamespace() (string, bool) {
	return os.LookupEnv("VELERO_NAMESPACE")
}
