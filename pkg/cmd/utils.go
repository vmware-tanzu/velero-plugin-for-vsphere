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
	"github.com/spf13/pflag"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/constants"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"os"
	"strconv"
	"strings"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"github.com/pkg/errors"
	"github.com/vmware-tanzu/velero/pkg/client"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func GetVeleroVersion(f client.Factory, ns string) (string, error) {
	clientset, err := f.KubeClient()
	if err != nil {
		fmt.Println("Failed to get kubeclient.")
		return "", err
	}
	deploymentList, err := clientset.AppsV1().Deployments(ns).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		fmt.Println("Failed to get deployment for velero namespace.")
		return "", err
	}
	for _, item := range deploymentList.Items {
		if item.GetName() == "velero" {
			version := GetVersionFromImage(item.Spec.Template.Spec.Containers, "velero/velero")
			return version, nil
		}
	}
	return "", nil
}

func GetVeleroFeatureFlags(f client.Factory, veleroNs string) ([]string, error) {
	var featureFlags = []string{}
	clientset, err := f.KubeClient()
	if err != nil {
		fmt.Println("Failed to get kubeclient.")
		return featureFlags, err
	}
	deploymentList, err := clientset.AppsV1().Deployments(veleroNs).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		fmt.Println("Failed to get deployment for velero namespace.")
		return featureFlags, err
	}
	for _, item := range deploymentList.Items {
		if item.GetName() == "velero" {
			featureFlags, err = GetFeatureFlagsFromImage(item.Spec.Template.Spec.Containers, "velero/velero")
			if err != nil {
				fmt.Println("ERROR: Failed to get feature flags for velero deployment.")
				return featureFlags, err
			}
			return featureFlags, nil
		}
	}
	return featureFlags, errors.Errorf("Unable to find velero deployment")
}

func GetFeatureFlagsFromImage(containers []v1.Container, imageName string) ([]string, error) {
	var containerArgs = []string{}
	for _, container := range containers {
		if strings.Contains(container.Image, imageName) {
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
	flags := pflag.CommandLine
	flags.StringVar(&featureString, "features", featureString, "list of feature flags for this plugin")
	flags.ParseErrorsWhitelist.UnknownFlags = true
	err := flags.Parse(containerArgs)
	if err != nil {
		fmt.Printf("WARNING: Error received while extracting feature flags: %v \n", err)
	}
	featureFlags := strings.Split(featureString, ",")
	return featureFlags, nil
}

func CreateFeatureStateConfigMap(features []string, f client.Factory, veleroNs string) error {
	ctx := context.Background()
	clientset, err := f.KubeClient()
	if err != nil {
		fmt.Println("Failed to get kubeclient.")
		return err
	}

	var create bool
	featureConfigMap, err := clientset.CoreV1().ConfigMaps(veleroNs).Get(ctx, constants.VSpherePluginFeatureStates, metav1.GetOptions{})
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
	if strings.Contains(featuresString, "EnableLocalMode") {
		featureData[constants.VSphereLocalModeFlag] = strconv.FormatBool(true)
	}
	featureConfigMap.Data = featureData
	if create {
		_, err = clientset.CoreV1().ConfigMaps(veleroNs).Create(ctx, featureConfigMap, metav1.CreateOptions{})
	} else {
		_, err = clientset.CoreV1().ConfigMaps(veleroNs).Update(ctx, featureConfigMap, metav1.UpdateOptions{})
	}
	if err != nil {
		fmt.Printf("Failed to create/update feature state config map : %s.\n", constants.VSpherePluginFeatureStates)
		return err
	}
	return nil
}

// Go doesn't have max function for integers?  Oh really, that's just so convenient
func max(x, y int) int {
	if x > y {
		return x
	}
	return y
}

// If currentVersion == minVersion, return 0
// If currentVersion > minVersion, return 1
// If currentVersion < minVersion, return -1
// Assume input string format is vX.Y.Z
func CompareVersion(currentVersion string, minVersion string) int {
	if currentVersion == "" {
		currentVersion = "v0.0.0"
	}
	if minVersion == "" {
		minVersion = "v0.0.0"
	}
	curVerArr := strings.Split(currentVersion[1:], ".")
	minVerArr := strings.Split(minVersion[1:], ".")

	for index := 0; index < max(len(curVerArr), len(minVerArr)); index++ {
		var curVerDigit, minVerDigit int
		if index < len(curVerArr) {
			curVerDigit, _ = strconv.Atoi(curVerArr[index])
		} else {
			curVerDigit = 0 // Substitute 0 if a position is missing
		}
		if index < len(minVerArr) {
			minVerDigit, _ = strconv.Atoi(minVerArr[index])
		} else {
			minVerDigit = 0
		}
		if curVerDigit < minVerDigit {
			return -1
		} else if curVerDigit > minVerDigit {
			return 1
		}
	}
	return 0
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

func CheckCSIInstalled(f client.Factory) (bool, bool, error) {
	clientset, err := f.KubeClient()
	if err != nil {
		return false, false, err
	}
	statefulsetList, err := clientset.AppsV1().StatefulSets("kube-system").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return false, false, err
	}
	for _, item := range statefulsetList.Items {
		if item.GetName() == "vsphere-csi-controller" {
			return CheckCSIVersion(item.Spec.Template.Spec.Containers)
		}
	}
	deploymentList, err := clientset.AppsV1().Deployments("kube-system").List(context.TODO(), metav1.ListOptions{})
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
