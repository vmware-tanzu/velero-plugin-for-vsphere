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

package cmd

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"github.com/spf13/pflag"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/constants"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/utils"
	"github.com/vmware-tanzu/velero/pkg/client"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"os"
	"strconv"
	"strings"
)

// CheckError prints err to stderr and exits with code 1 if err is not nil. Otherwise, it is a
// no-op.
func CheckError(err error) {
	if err != nil {
		if err != context.Canceled {
			fmt.Fprintf(os.Stderr, "An error occurred: %v\n", err)
		}
		os.Exit(1)
	}
}

// Exit prints msg (with optional args), plus a newline, to stderr and exits with code 1.
func Exit(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, msg+"\n", args...)
	os.Exit(1)
}

func GetVeleroVersion(kubeClient kubernetes.Interface, ns string) (string, error) {
	veleroDeployment, err := kubeClient.AppsV1().Deployments(ns).Get(context.TODO(), constants.VeleroDeployment, metav1.GetOptions{})
	if err != nil {
		fmt.Println("Failed to get deployment for velero namespace.")
		return "", err
	}
	versionTag := utils.GetVersionFromImage(veleroDeployment.Spec.Template.Spec.Containers, "velero")
	if versionTag == "" {
		fmt.Printf("Failed to get tag from velero image\n")
	}
	return versionTag, nil
}

func GetVeleroFeatureFlags(kubeClient kubernetes.Interface, ns string) ([]string, error) {
	var featureFlags = []string{}

	veleroDeployment, err := kubeClient.AppsV1().Deployments(ns).Get(context.TODO(), constants.VeleroDeployment, metav1.GetOptions{})
	if err != nil {
		fmt.Println("Failed to get deployment for velero namespace.")
		return featureFlags, err
	}

	featureFlags, err = GetFeatureFlagsFromImage(veleroDeployment.Spec.Template.Spec.Containers, "velero")
	if err != nil {
		fmt.Println("Failed to get feature flags for velero deployment.")
		return featureFlags, err
	}

	return featureFlags, nil
}

func GetFeatureFlagsFromImage(containers []v1.Container, containerName string) ([]string, error) {
	var containerArgs = []string{}
	for _, container := range containers {
		if containerName == container.Name {
			containerArgs = container.Args[1:]
			break
		}
	}
	if len(containerArgs) == 0 {
		fmt.Printf("No arguments found, no feature flags detected.")
		return []string{}, nil
	}
	if len(containerArgs) > 1 {
		fmt.Printf("Unexpected container arguments for velero image will be using only the first: %v \n", containerArgs[1])
	}
	// Extract the flags from the server feature flag args.
	var featureString string
	flags := pflag.NewFlagSet("velero-container-command-flags", pflag.ExitOnError)
	flags.StringVar(&featureString, "features", featureString, "list of feature flags for this plugin")
	flags.ParseErrorsWhitelist.UnknownFlags = true
	err := flags.Parse(containerArgs)
	if err != nil {
		fmt.Printf("WARNING: Error received while extracting feature flags: %v \n", err)
	}
	featureFlags := strings.Split(featureString, ",")
	return featureFlags, nil
}

func CreateFeatureStateConfigMap(kubeClient kubernetes.Interface, features []string, veleroNs string) error {
	ctx := context.Background()

	var create bool
	featureConfigMap, err := kubeClient.CoreV1().ConfigMaps(veleroNs).Get(ctx, constants.VSpherePluginFeatureStates, metav1.GetOptions{})
	var featureData map[string]string
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			fmt.Printf("Failed to retrieve %s configuration.\n", constants.VSpherePluginFeatureStates)
			return err
		}
		featureConfigMap = &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      constants.VSpherePluginFeatureStates,
				Namespace: veleroNs,
			},
			Data: make(map[string]string),
		}
		create = true
	}
	//Always overwrite the feature flags.
	featureData = make(map[string]string)
	// Insert the keys with default values.
	featureData[constants.VSphereLocalModeFlag] = strconv.FormatBool(false)
	// Update the falgs based on velero feature flags.
	featuresString := strings.Join(features[:], ",")
	if strings.Contains(featuresString, constants.VSphereLocalModeFeature) {
		featureData[constants.VSphereLocalModeFlag] = strconv.FormatBool(true)
	}
	// Update the data to the default if the flag is not found, the default is true.
	if decoupleVSphereCSIDriverFlag, ok := featureConfigMap.Data[constants.DecoupleVSphereCSIDriverFlag]; !ok {
		featureData[constants.DecoupleVSphereCSIDriverFlag] = strconv.FormatBool(true)
	} else {
		featureData[constants.DecoupleVSphereCSIDriverFlag] = decoupleVSphereCSIDriverFlag
	}

	if migratedVolumeSupportFlag, ok := featureConfigMap.Data[constants.CSIMigratedVolumeSupportFlag]; !ok {
		featureData[constants.CSIMigratedVolumeSupportFlag] = strconv.FormatBool(false)
	} else {
		featureData[constants.CSIMigratedVolumeSupportFlag] = migratedVolumeSupportFlag
	}

	featureConfigMap.Data = featureData
	if create {
		_, err = kubeClient.CoreV1().ConfigMaps(veleroNs).Create(ctx, featureConfigMap, metav1.CreateOptions{})
	} else {
		_, err = kubeClient.CoreV1().ConfigMaps(veleroNs).Update(ctx, featureConfigMap, metav1.UpdateOptions{})
	}
	if err != nil {
		fmt.Printf("Failed to create/update feature state config map : %s.\n", constants.VSpherePluginFeatureStates)
		return err
	}
	return nil
}

func BuildConfig(master, kubeConfig string, f client.Factory) (*rest.Config, error) {
	var config *rest.Config
	var err error
	if master != "" || kubeConfig != "" {
		config, err = clientcmd.BuildConfigFromFlags(master, kubeConfig)
	} else {
		config, err = f.ClientConfig()
	}
	if err != nil {
		return nil, errors.Errorf("failed to create config: %v", err)
	}
	return config, nil
}

func GetCompatibleRepoAndTagFromPluginImage(kubeClient kubernetes.Interface, namespace string, targetContainer string) (string, error) {
	deployment, err := kubeClient.AppsV1().Deployments(namespace).Get(context.TODO(), constants.VeleroDeployment, metav1.GetOptions{})
	if err != nil {
		return "", errors.Errorf("Failed to get velero deployment in namespace %s", namespace)
	}

	var repo, tag, image string
	for _, container := range deployment.Spec.Template.Spec.InitContainers {
		imageContainer := utils.GetComponentFromImage(container.Image, constants.ImageContainerComponent)
		if imageContainer == constants.VeleroPluginForVsphere {
			image = container.Image
			repo = utils.GetComponentFromImage(image, constants.ImageRepositoryComponent)
			tag = utils.GetComponentFromImage(image, constants.ImageVersionComponent)
			break
		}
	}

	if image == "" {
		return "", errors.New("The plugin, velero-plugin-for-vsphere, was not added as an init container of Velero deployment")
	}

	resultImage := targetContainer
	if repo != "" {
		resultImage = repo + "/" + resultImage
	}
	if tag != "" {
		resultImage = resultImage + ":" + tag
	}
	return resultImage, nil
}

func CheckVSphereCSIDriverVersion(kubeClient kubernetes.Interface, clusterFlavor constants.ClusterFlavor) error {
	if clusterFlavor != constants.VSphere {
		fmt.Println("Skipped the version check of CSI driver if it is not in a Vanilla cluster")
		return nil
	}

	csiInstalledVersion, err := utils.GetCSIInstalledVersion(kubeClient)
	if err != nil {
		fmt.Printf("Failed the version check of CSI driver. Error: %v\n", err)
		return err
	}
	// Current minimum version support v1.0.2
	if utils.CompareVersion(csiInstalledVersion, constants.CsiMinVersion) < 0 {
		return errors.Errorf("The version of vSphere CSI controller %s is below the minimum requirement v1.0.2", csiInstalledVersion)
	}

	return nil
}

func CheckVeleroVersion(kubeClient kubernetes.Interface, ns string) error {
	veleroVersion, err := GetVeleroVersion(kubeClient, ns)
	if err != nil || veleroVersion == "" {
		fmt.Println("Failed to get velero version.")
	} else {
		if utils.CompareVersion(veleroVersion, constants.VeleroMinVersion) == -1 {
			fmt.Printf("WARNING: Velero version %s is prior to %s. Velero Plug-in for vSphere requires velero version to be %s or above.\n", veleroVersion, constants.VeleroMinVersion, constants.VeleroMinVersion)
		}
	}

	return nil
}

func CheckPluginImageRepo(kubeClient kubernetes.Interface, ns string, defaultImage string, serverType string) (string, error) {
	resultImage, err := GetCompatibleRepoAndTagFromPluginImage(kubeClient, ns, serverType)
	if err != nil {
		resultImage = defaultImage
		fmt.Printf("Failed to check plugin image repo, error msg: %s. Using default image %s\n", err.Error(), resultImage)
	} else {
		fmt.Printf("Using image %s\n", resultImage)
	}

	return resultImage, err
}


// If there is no configmap velero-vsphere-plugin-block-list created before, create a new configmap from the default blocking list.
// If there is already a configmap velero-vsphere-plugin-block-list created, leave the previous one unchanged.
func CreateBlockListConfigMap(kubeClient kubernetes.Interface, veleroNs string, resourceToBlock map[string]bool) error {
	ctx := context.Background()
	var create bool
	blockListConfigMap, err := kubeClient.CoreV1().ConfigMaps(veleroNs).Get(ctx, constants.ResourcesToBlockListName, metav1.GetOptions{})
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			fmt.Printf("Failed to retrieve %s configuration.\n", constants.ResourcesToBlockListName)
			return err
		}
		blockListConfigMap = &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      constants.ResourcesToBlockListName,
				Namespace: veleroNs,
			},
			Data: make(map[string]string),
		}
		create = true
	}

	// If there is blocking list configmap created already, leave the previous one unchanged.
	if !create {
		fmt.Print("Config map that blocks resources for backup/restore already exists. Using the existing one.")
		return nil
	}

	blockListData := make(map[string]string)
	for resourceToBlock, isBlock := range resourceToBlock {
		blockListData[resourceToBlock] = strconv.FormatBool(isBlock)
	}
	blockListConfigMap.Data = blockListData
	_, err = kubeClient.CoreV1().ConfigMaps(veleroNs).Create(ctx, blockListConfigMap, metav1.CreateOptions{})
	if err != nil {
		fmt.Printf("failed to create the config map %s that blocks resources for backup and restore.\n", constants.ResourcesToBlockListName)
		return err
	}

	return nil
}

func RetrieveBlockListConfigMap() (map[string]string, error) {
	ctx := context.Background()

	veleroNs, exist := os.LookupEnv("VELERO_NAMESPACE")
	if !exist {
		errMsg := "Failed to lookup the ENV variable for velero namespace"
		return nil, errors.New(errMsg)
	}

	restConfig, err := utils.GetKubeClientConfig()
	if err != nil {
		return nil, errors.WithStack(err)
	}

	kubeClient, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	blockListConfigMap, err := kubeClient.CoreV1().ConfigMaps(veleroNs).Get(ctx, constants.ResourcesToBlockListName, metav1.GetOptions{})
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			fmt.Printf("Failed to retrieve configmap: %s. Error: %v\n", constants.ResourcesToBlockListName, err)
		} else {
			fmt.Printf("Failed to find configmap: %s. Error: %v\n", constants.ResourcesToBlockListName, err)
		}
		return nil, err
	}

	return blockListConfigMap.Data, nil
}
