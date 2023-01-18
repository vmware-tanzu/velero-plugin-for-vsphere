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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func WithEnvFromSecretKey(varName, secret, key string) podTemplateOption {
	return func(c *podTemplateConfig) {
		c.envVars = append(c.envVars, corev1.EnvVar{
			Name: varName,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: secret,
					},
					Key: key,
				},
			},
		})
	}
}

func Deployment(namespace string, opts ...podTemplateOption) *appsv1.Deployment {
	c := &podTemplateConfig{
		image: DefaultBackupDriverImage,
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

	deployment := &appsv1.Deployment{
		ObjectMeta: objectMeta(namespace, "backup-driver"),
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: appsv1.SchemeGroupVersion.String(),
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": "backup-driver",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"name":      "backup-driver",
						"component": "velero",
					},
					Annotations: c.annotations,
				},
				Spec: corev1.PodSpec{
					RestartPolicy:      corev1.RestartPolicyAlways,
					ServiceAccountName: "velero",
					SecurityContext: &corev1.PodSecurityContext{
						RunAsUser: &userID,
					},
					Volumes: []corev1.Volume{
						{
							Name: "scratch",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: new(corev1.EmptyDirVolumeSource),
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:            "backup-driver",
							Image:           c.image,
							Ports:           containerPorts(),
							ImagePullPolicy: pullPolicy,
							Command: []string{
								"/backup-driver",
							},
							Args: []string{
								"server",
							},
							VolumeMounts: []corev1.VolumeMount{
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

	if c.masterAffinity {
		deployment.Spec.Template.Spec.Tolerations = append(
			deployment.Spec.Template.Spec.Tolerations,
			corev1.Toleration{
				Effect:   "NoSchedule",
				Key:      "node-role.kubernetes.io/master",
				Operator: "Exists",
			},
			corev1.Toleration{
				Effect:   "NoSchedule",
				Key:      "node-role.kubernetes.io/control-plane",
				Operator: "Exists",
			},
			corev1.Toleration{
				Effect:   "NoSchedule",
				Key:      "kubeadmNode",
				Operator: "Equal",
				Value:    "master",
			},
		)

		deployment.Spec.Template.Spec.NodeSelector = make(map[string]string)
		deployment.Spec.Template.Spec.NodeSelector["node-role.kubernetes.io/control-plane"] = ""
	}

	if c.hostNetwork {
		deployment.Spec.Template.Spec.HostNetwork = true
	}

	deployment.Spec.Template.Spec.Containers[0].Env = append(deployment.Spec.Template.Spec.Containers[0].Env, c.envVars...)

	return deployment
}
