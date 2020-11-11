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
	"github.com/hashicorp/go-version"
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

// Return version in the format: vX.Y.Z
func GetVersionFromImage(containers []v1.Container, imageName string) string {
	var tag = ""
	for _, container := range containers {
		if strings.Contains(container.Image, imageName) {
			tag = strings.Split(container.Image, ":")[1]
			break
		}
	}
	if tag == "" {
		fmt.Printf("Failed to get tag from image %s\n", imageName)
		return ""
	}
	if strings.Contains(tag, "-") {
		version := strings.Split(tag, "-")[0]
		return version
	} else {
		return tag
	}
}

// Return version in the format: vX.Y.Z
func GetVersionFromImageByContainerName(containers []v1.Container, containerName string) string {
	var tag string
	for _, container := range containers {
		if containerName == container.Name && containerName == utils.GetComponentFromImage(container.Image, constants.ImageContainerComponent) {
			tag = utils.GetComponentFromImage(container.Image, constants.ImageVersionComponent)
			break
		}
	}
	if tag == "" {
		fmt.Printf("Failed to get tag from image %s\n", containerName)
	}

	return tag
}

func GetVeleroVersion(kubeClient kubernetes.Interface, ns string) (string, error) {
	veleroDeployment, err := kubeClient.AppsV1().Deployments(ns).Get(context.TODO(), constants.VeleroDeployment, metav1.GetOptions{})
	if err != nil {
		fmt.Println("Failed to get deployment for velero namespace.")
		return "", err
	}

	return GetVersionFromImageByContainerName(veleroDeployment.Spec.Template.Spec.Containers, "velero"), nil
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
		if containerName == container.Name && containerName == utils.GetComponentFromImage(container.Image, constants.ImageContainerComponent) {
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

// If currentVersion < minVersion, return -1
// If currentVersion == minVersion, return 0
// If currentVersion > minVersion, return 1
// Assume input versions are both valid
func CompareVersion(currentVersion string, minVersion string) int {
	current, _ := version.NewVersion(currentVersion)
	minimum, _ := version.NewVersion(minVersion)

	return current.Compare(minimum)
}

func CheckCSIVersion(containers []v1.Container) (bool, bool, error) {
	isVersionOK := false
	csi_driver_version := GetVersionFromImage(containers, "cloud-provider-vsphere/csi/release/driver")
	if csi_driver_version == "" {
		csi_driver_version = GetVersionFromImage(containers, "cloudnativestorage/vsphere-csi")
		if csi_driver_version != "" {
			fmt.Printf("Got pre-relase version %s from container cloudnativestorage/vsphere-csi, setting version to min version %s\n",
				csi_driver_version, constants.CsiMinVersion)
			csi_driver_version = constants.CsiMinVersion
		}
	}
	csi_syncer_version := GetVersionFromImage(containers, "cloud-provider-vsphere/csi/release/syncer")
	if csi_syncer_version == "" {
		csi_syncer_version = GetVersionFromImage(containers, "cloudnativestorage/syncer")
		if csi_syncer_version != "" {
			fmt.Printf("Got pre-relase version %s from container cloudnativestorage/syncer, setting version to min version %s\n",
				csi_syncer_version, constants.CsiMinVersion)
			csi_syncer_version = constants.CsiMinVersion
		}
	}
	if CompareVersion(csi_driver_version, constants.CsiMinVersion) >= 0 && CompareVersion(csi_syncer_version, constants.CsiMinVersion) >= 0 {
		isVersionOK = true
	}
	return true, isVersionOK, nil
}

func CheckCSIInstalled(kubeClient kubernetes.Interface) (bool, bool, error) {
	statefulsetList, err := kubeClient.AppsV1().StatefulSets("kube-system").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return false, false, err
	}
	for _, item := range statefulsetList.Items {
		if item.GetName() == "vsphere-csi-controller" {
			return CheckCSIVersion(item.Spec.Template.Spec.Containers)
		}
	}
	deploymentList, err := kubeClient.AppsV1().Deployments("kube-system").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return false, false, err
	}
	for _, item := range deploymentList.Items {
		if item.Name == "vsphere-csi-controller" {
			return CheckCSIVersion(item.Spec.Template.Spec.Containers)
		}
	}
	return false, false, nil
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

func CheckVSphereCSIDriverVersion(kubeClient kubernetes.Interface) error {
	isCSIInstalled, isVersionOk, err := CheckCSIInstalled(kubeClient)
	if err != nil {
		fmt.Println("CSI driver check failed")
		isCSIInstalled = false
		isVersionOk = false
	}

	if !isCSIInstalled {
		fmt.Println("Velero Plug-in for vSphere requires vSphere CSI/CNS and vSphere 6.7U3 to function. Please install the vSphere CSI/CNS driver")
	}

	if !isVersionOk {
		fmt.Printf("vSphere CSI driver version is prior to %s. Velero Plug-in for vSphere requires CSI driver version to be %s or above\n", constants.CsiMinVersion, constants.CsiMinVersion)
	}

	return err
}

func CheckVeleroVersion(kubeClient kubernetes.Interface, ns string) error {
	veleroVersion, err := GetVeleroVersion(kubeClient, ns)
	if err != nil || veleroVersion == "" {
		fmt.Println("Failed to get velero version.")
	} else {
		if CompareVersion(veleroVersion, constants.VeleroMinVersion) == -1 {
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
