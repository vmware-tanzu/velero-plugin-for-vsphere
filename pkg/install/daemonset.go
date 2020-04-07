/*
Copyright 2018, 2019 the Velero contributors.

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
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type podTemplateOption func(*podTemplateConfig)

type podTemplateConfig struct {
	image                             string
	envVars                           []corev1.EnvVar
	restoreOnly                       bool
	annotations                       map[string]string
	resources                         corev1.ResourceRequirements
	withSecret                        bool
	defaultResticMaintenanceFrequency time.Duration
	plugins                           []string
}

func WithImage(image string) podTemplateOption {
	return func(c *podTemplateConfig) {
		c.image = image
	}
}

func WithAnnotations(annotations map[string]string) podTemplateOption {
	return func(c *podTemplateConfig) {
		c.annotations = annotations
	}
}

func WithSecret(secretPresent bool) podTemplateOption {
	return func(c *podTemplateConfig) {
		c.withSecret = secretPresent

	}
}

func WithRestoreOnly() podTemplateOption {
	return func(c *podTemplateConfig) {
		c.restoreOnly = true
	}
}

func WithResources(resources corev1.ResourceRequirements) podTemplateOption {
	return func(c *podTemplateConfig) {
		c.resources = resources
	}
}

func WithDefaultResticMaintenanceFrequency(val time.Duration) podTemplateOption {
	return func(c *podTemplateConfig) {
		c.defaultResticMaintenanceFrequency = val
	}
}

func DaemonSet(namespace string, opts ...podTemplateOption) *appsv1.DaemonSet {
	c := &podTemplateConfig{
		image: DefaultImage,
	}

	for _, opt := range opts {
		opt(c)
	}

	pullPolicy := corev1.PullAlways
	imageParts := strings.Split(c.image, ":")
	if len(imageParts) == 2 && imageParts[1] != "latest" {
		pullPolicy = corev1.PullIfNotPresent

	}

	userID := int64(0)

	daemonSet := &appsv1.DaemonSet{
		ObjectMeta: objectMeta(namespace, "datamgr-for-vsphere-plugin"),
		TypeMeta: metav1.TypeMeta{
			Kind:       "DaemonSet",
			APIVersion: appsv1.SchemeGroupVersion.String(),
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": "datamgr-for-vsphere-plugin",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"name":      "datamgr-for-vsphere-plugin",
						"component": "velero",
					},
					Annotations: c.annotations,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: "velero-server",
					SecurityContext: &corev1.PodSecurityContext{
						RunAsUser: &userID,
					},
					Volumes: []corev1.Volume{
						{
							Name: "plugins",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
						{
							Name: "scratch",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: new(corev1.EmptyDirVolumeSource),
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:            "datamgr-for-vsphere-plugin",
							Image:           c.image,
							ImagePullPolicy: pullPolicy,
							Command: []string{
								"/datamgr",
							},
							Args: []string{
								"server",
							},

							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "plugins",
									MountPath: "/plugins",
								},
								{
									Name:      "scratch",
									MountPath: "/scratch",
								},
							},
							Env: []corev1.EnvVar{
								{
									Name: "NODE_NAME",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "spec.nodeName",
										},
									},
								},
								{
									Name: "VELERO_NAMESPACE",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "metadata.namespace",
										},
									},
								},
								{
									Name:  "VELERO_SCRATCH_DIR",
									Value: "/scratch",
								},
								{
									Name:  "LD_LIBRARY_PATH",
									Value: "/vddkLibs",
								},
							},
							Resources: c.resources,
						},
					},
				},
			},
		},
	}

	if c.withSecret {
		daemonSet.Spec.Template.Spec.Volumes = append(
			daemonSet.Spec.Template.Spec.Volumes,
			corev1.Volume{
				Name: "cloud-credentials",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: "velero",
					},
				},
			},
		)

		daemonSet.Spec.Template.Spec.Containers[0].VolumeMounts = append(
			daemonSet.Spec.Template.Spec.Containers[0].VolumeMounts,
			corev1.VolumeMount{
				Name:      "cloud-credentials",
				MountPath: "/credentials",
			},
		)

		daemonSet.Spec.Template.Spec.Containers[0].Env = append(daemonSet.Spec.Template.Spec.Containers[0].Env, []corev1.EnvVar{
			{
				Name:  "GOOGLE_APPLICATION_CREDENTIALS",
				Value: "/credentials/cloud",
			},
			{
				Name:  "AWS_SHARED_CREDENTIALS_FILE",
				Value: "/credentials/cloud",
			},
			{
				Name:  "AZURE_CREDENTIALS_FILE",
				Value: "/credentials/cloud",
			},
		}...)
	}

	daemonSet.Spec.Template.Spec.Containers[0].Env = append(daemonSet.Spec.Template.Spec.Containers[0].Env, c.envVars...)

	return daemonSet
}
