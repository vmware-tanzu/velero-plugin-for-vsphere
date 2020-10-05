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

package install

import (
	"fmt"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/constants"
	"io"
	"strings"
	"time"

	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated/crds"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/pkg/errors"
	"github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/buildinfo"
	"github.com/vmware-tanzu/velero/pkg/client"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// kindToResource translates a Kind (mixed case, singular) to a Resource (lowercase, plural) string.
// This is to accomodate the dynamic client's need for an APIResource, as the Unstructured objects do not have easy helpers for this information.
var kindToResource = map[string]string{
	"CustomResourceDefinition": "customresourcedefinitions",
	"Namespace":                "namespaces",
	"ClusterRoleBinding":       "clusterrolebindings",
	"ServiceAccount":           "serviceaccounts",
	"Deployment":               "deployments",
	"DaemonSet":                "daemonsets",
	"Secret":                   "secrets",
	"BackupStorageLocation":    "backupstoragelocations",
	"VolumeSnapshotLocation":   "volumesnapshotlocations",
}

// ResourceGroup represents a collection of kubernetes objects with a common ready conditon
type ResourceGroup struct {
	CRDResources   []*unstructured.Unstructured
	OtherResources []*unstructured.Unstructured
}

// TODO: Update this list with plugin CRDs
var CRDsList = []string{
	"backupstoragelocations.velero.io",
	"volumesnapshotlocations.velero.io",
}

// DefaultImage is the default image to use for the Velero deployment and restic daemonset containers.
var (
	DefaultDatamgrImage          = imageRegistry() + "/" + constants.DataManagerForPlugin + ":" + imageVersion()
	DefaultDatamgrPodCPURequest  = "0"
	DefaultDatamgrPodMemRequest  = "0"
	DefaultDatamgrPodCPULimit    = "0"
	DefaultDatamgrPodMemLimit    = "0"

	DefaultBackupDriverImage          = imageRegistry() + "/" + constants.BackupDriverForPlugin + ":" + imageVersion()
	DefaultBackupDriverPodCPURequest  = "0"
	DefaultBackupDriverPodMemRequest  = "0"
	DefaultBackupDriverPodCPULimit    = "0"
	DefaultBackupDriverPodMemLimit    = "0"
)

type PodOptions struct {
	Namespace      string
	Image          string
	ProviderName   string
	Bucket         string
	Prefix         string
	PodAnnotations map[string]string
	PodResources   corev1.ResourceRequirements
	SecretData     []byte
	SecretAdd      bool
	MasterAffinity bool
	HostNetwork    bool
	Features	   []string
}

// Use "latest" if the build process didn't supply a version
func imageVersion() string {
	if buildinfo.Version == "" {
		return "latest"
	}
	return buildinfo.Version
}

func imageRegistry() string {
	if buildinfo.Registry == "" {
		return "dpcpinternal"
	}
	return buildinfo.Registry
}

func labels() map[string]string {
	return map[string]string{
		"component": "velero",
	}
}

func objectMeta(namespace, name string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      name,
		Namespace: namespace,
		Labels:    labels(),
	}
}

func containerPorts() []corev1.ContainerPort {
	return []corev1.ContainerPort{
		{
			Name:          "metrics",
			ContainerPort: 8085,
		},
	}
}

// createResource attempts to create a resource in the cluster.
// If the resource already exists in the cluster, it's merely logged.
func createResource(r *unstructured.Unstructured, factory client.DynamicFactory, w io.Writer) error {
	id := fmt.Sprintf("%s/%s", r.GetKind(), r.GetName())

	// Helper to reduce boilerplate message about the same object
	log := func(f string, a ...interface{}) {
		format := strings.Join([]string{id, ": ", f, "\n"}, "")
		fmt.Fprintf(w, format, a...)
	}
	log("attempting to create resource")

	gvk := schema.FromAPIVersionAndKind(r.GetAPIVersion(), r.GetKind())

	apiResource := metav1.APIResource{
		Name:       kindToResource[r.GetKind()],
		Namespaced: (r.GetNamespace() != ""),
	}

	c, err := factory.ClientForGroupVersionResource(gvk.GroupVersion(), apiResource, r.GetNamespace())
	if err != nil {
		return errors.Wrapf(err, "Error creating client for resource %s", id)
	}

	if _, err := c.Create(r); apierrors.IsAlreadyExists(err) {
		log("already exists, proceeding")
	} else if err != nil {
		return errors.Wrapf(err, "Error creating resource %s", id)
	}

	log("created")
	return nil
}

// crdIsReady checks a CRD to see if it's ready, so that objects may be created from it.
func crdIsReady(crd *apiextv1beta1.CustomResourceDefinition) bool {
	var isEstablished, namesAccepted bool
	for _, cond := range crd.Status.Conditions {
		if cond.Type == apiextv1beta1.Established {
			isEstablished = true
		}
		if cond.Type == apiextv1beta1.NamesAccepted {
			namesAccepted = true
		}
	}

	return (isEstablished && namesAccepted)
}

// crdsAreReady polls the API server to see if the BackupStorageLocation and VolumeSnapshotLocation CRDs are ready to create objects.
func crdsAreReady(factory client.DynamicFactory, crdKinds []string) (bool, error) {
	gvk := schema.FromAPIVersionAndKind(apiextv1beta1.SchemeGroupVersion.String(), "CustomResourceDefinition")
	apiResource := metav1.APIResource{
		Name:       kindToResource["CustomResourceDefinition"],
		Namespaced: false,
	}
	c, err := factory.ClientForGroupVersionResource(gvk.GroupVersion(), apiResource, "")
	if err != nil {
		return false, errors.Wrapf(err, "Error creating client for CustomResourceDefinition polling")
	}
	// Track all the CRDs that have been found and successfully marshalled.
	// len should be equal to len(crdKinds) in the happy path.
	foundCRDs := make([]*apiextv1beta1.CustomResourceDefinition, 0)
	var areReady bool
	err = wait.PollImmediate(time.Second, time.Minute, func() (bool, error) {
		for _, k := range crdKinds {
			unstruct, err := c.Get(k, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				return false, nil
			} else if err != nil {
				return false, errors.Wrapf(err, "error waiting for %s to be ready", k)
			}

			crd := new(apiextv1beta1.CustomResourceDefinition)
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstruct.Object, crd); err != nil {
				return false, errors.Wrapf(err, "error converting %s from unstructured", k)
			}

			foundCRDs = append(foundCRDs, crd)
		}

		if len(foundCRDs) != len(crdKinds) {
			return false, nil
		}

		for _, crd := range foundCRDs {
			if !crdIsReady(crd) {
				return false, nil
			}

		}
		areReady = true

		return true, nil
	})
	return areReady, nil
}

func appendUnstructured(list *unstructured.UnstructuredList, obj runtime.Object) error {
	u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&obj)

	// Remove the status field so we're not sending blank data to the server.
	// On CRDs, having an empty status is actually a validation error.
	delete(u, "status")
	if err != nil {
		return err
	}
	list.Items = append(list.Items, unstructured.Unstructured{Object: u})
	return nil
}

func isAvailable(c appsv1.DeploymentCondition) bool {
	// Make sure that the deployment has been available for at least 10 seconds.
	// This is because the deployment can show as Ready momentarily before the pods fall into a CrashLoopBackOff.
	// See podutils.IsPodAvailable upstream for similar logic with pods
	if c.Type == appsv1.DeploymentAvailable && c.Status == corev1.ConditionTrue {
		if !c.LastTransitionTime.IsZero() && c.LastTransitionTime.Add(10*time.Second).Before(time.Now()) {
			return true
		}
	}
	return false
}

// DeploymentIsReady will poll the kubernetes API server to see if the backup-driver deployment is ready to service user requests.
func DeploymentIsReady(factory client.DynamicFactory, namespace string) (bool, error) {
	gvk := schema.FromAPIVersionAndKind(appsv1.SchemeGroupVersion.String(), "Deployment")
	apiResource := metav1.APIResource{
		Name:       "deployments",
		Namespaced: true,
	}
	c, err := factory.ClientForGroupVersionResource(gvk.GroupVersion(), apiResource, namespace)
	if err != nil {
		return false, errors.Wrapf(err, "Error creating client for deployment polling")
	}
	// declare this variable out of scope so we can return it
	var isReady bool
	var readyObservations int32
	err = wait.PollImmediate(time.Second, time.Minute, func() (bool, error) {
		unstructuredDeployment, err := c.Get("backup-driver", metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return false, nil
		} else if err != nil {
			return false, errors.Wrap(err, "error waiting for deployment to be ready")
		}

		deploy := new(appsv1.Deployment)
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredDeployment.Object, deploy); err != nil {
			return false, errors.Wrap(err, "error converting deployment from unstructured")
		}

		for _, cond := range deploy.Status.Conditions {
			if isAvailable(cond) {
				readyObservations++
			}
		}
		// Make sure we query the deployment enough times to see the state change, provided there is one.
		if readyObservations > 4 {
			isReady = true
			return true, nil
		} else {
			return false, nil
		}
	})
	return isReady, err
}

// DaemonSetIsReady will poll the kubernetes API server to ensure the datamgr daemonset is ready, i.e. that
// pods are scheduled and available on all of the the desired nodes.
func DaemonSetIsReady(factory client.DynamicFactory, namespace string, nNodes int) (bool, error) {
	gvk := schema.FromAPIVersionAndKind(appsv1.SchemeGroupVersion.String(), "DaemonSet")
	apiResource := metav1.APIResource{
		Name:       "daemonsets",
		Namespaced: true,
	}

	c, err := factory.ClientForGroupVersionResource(gvk.GroupVersion(), apiResource, namespace)
	if err != nil {
		return false, errors.Wrapf(err, "Error creating client for daemonset polling")
	}

	// declare this variable out of scope so we can return it
	var isReady bool
	var readyObservations int32
	timeout := time.Duration(nNodes) * time.Minute

	err = wait.PollImmediate(time.Second, timeout, func() (bool, error) {
		unstructuredDaemonSet, err := c.Get("datamgr-for-vsphere-plugin", metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return false, nil
		} else if err != nil {
			return false, errors.Wrap(err, "error waiting for daemonset to be ready")
		}

		daemonSet := new(appsv1.DaemonSet)
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredDaemonSet.Object, daemonSet); err != nil {
			return false, errors.Wrap(err, "error converting daemonset from unstructured")
		}

		if daemonSet.Status.NumberAvailable == daemonSet.Status.DesiredNumberScheduled {
			readyObservations++
		}

		// Wait for 5 observations of the daemonset being "ready" to be consistent with our check for
		// the deployment being ready.
		if readyObservations > 4 {
			isReady = true
			return true, nil
		} else {
			return false, nil
		}
	})
	return isReady, err
}

// TODO: Split CRDs, or only install when backup driver is installed?
func AllCRDs() *unstructured.UnstructuredList {
	resources := new(unstructured.UnstructuredList)
	// Set the GVK so that the serialization framework outputs the list properly
	resources.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "List"})

	for _, crd := range crds.CRDs {
		crd.SetLabels(labels())
		appendUnstructured(resources, crd)
	}

	return resources
}

// AllDatamgrResources returns a list of all resources necessary to install Datamgr, in the appropriate order, into a Kubernetes cluster.
// Items are unstructured, since there are different data types returned.
func AllDatamgrResources(o *PodOptions, withCRDs bool) (*unstructured.UnstructuredList, error) {
	var resources *unstructured.UnstructuredList
	if withCRDs {
		resources = AllCRDs()
	} else {
		resources = new(unstructured.UnstructuredList)
	}

	// Datamgr pod
	ds := DaemonSet(o.Namespace,
		WithAnnotations(o.PodAnnotations),
		WithImage(o.Image),
		WithResources(o.PodResources),
		WithSecret(o.SecretAdd),
	)
	appendUnstructured(resources, ds)

	return resources, nil
}

// AllBackupDriverResources returns a list of all resources necessary to install Datamgr, in the appropriate order, into a Kubernetes cluster.
// Items are unstructured, since there are different data types returned.
func AllBackupDriverResources(o *PodOptions, withCRDs bool) (*unstructured.UnstructuredList, error) {
	var resources *unstructured.UnstructuredList
	if withCRDs {
		resources = AllCRDs()
	} else {
		resources = new(unstructured.UnstructuredList)
	}

	// BackupDriver pod
	deploy := Deployment(o.Namespace,
		WithAnnotations(o.PodAnnotations),
		WithImage(o.Image),
		WithResources(o.PodResources),
		WithMasterNodeAffinity(o.MasterAffinity),
		WithHostNetwork(o.HostNetwork),
	)
	appendUnstructured(resources, deploy)

	return resources, nil
}

// GroupResources groups resources based on whether the resources are CustomResourceDefinitions or other types of kubernetes objects
// This is useful to wait for readiness before creating CRD objects
func GroupResources(resources *unstructured.UnstructuredList) *ResourceGroup {
	rg := new(ResourceGroup)

	for i, r := range resources.Items {
		if r.GetKind() == "CustomResourceDefinition" {
			rg.CRDResources = append(rg.CRDResources, &resources.Items[i])
			continue
		}
		rg.OtherResources = append(rg.OtherResources, &resources.Items[i])
	}

	return rg
}

// Install creates resources on the Kubernetes cluster.
// An unstructured list of resources is sent, one at a time, to the server. These are assumed to be in the preferred order already.
// Resources will be sorted into CustomResourceDefinitions and any other resource type, and the function will wait up to 1 minute
// for CRDs to be ready before proceeding.
// An io.Writer can be used to output to a log or the console.
func Install(factory client.DynamicFactory, resources *unstructured.UnstructuredList, w io.Writer) error {
	rg := GroupResources(resources)

	//Install CRDs first
	for _, r := range rg.CRDResources {
		if err := createResource(r, factory, w); err != nil {
			return err
		}
	}

	// Wait for CRDs to be ready before proceeding
	fmt.Fprint(w, "Waiting for resources to be ready in cluster...\n")
	_, err := crdsAreReady(factory, CRDsList)
	if err == wait.ErrWaitTimeout {
		return errors.Errorf("timeout reached, CRDs not ready")
	} else if err != nil {
		return err
	}

	// Install all other resources
	for _, r := range rg.OtherResources {
		if err = createResource(r, factory, w); err != nil {
			return err
		}
	}

	return nil
}
