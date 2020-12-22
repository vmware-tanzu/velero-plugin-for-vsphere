# Velero Plugin for vSphere Known Issues

This section lists the major known issues with Velero Plugin for vSphere. For a complete list of issues, check the [Github issues](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/issues) page. If you notice an issue not listed in Github issues page, please file an issue on the Github repository.

## vSphere CSI driver config file

On a Vanilla cluster, Velero Plugin for vSphere expects the key `csi-vsphere.conf` being presented in the `data` field of the secret `vsphere-config-secret`. If it is not present, backup will fail. The secret name `vsphere-config-secret` and the key `csi-vsphere.conf` are default names when installing vSphere CSI driver.

```
kubectl get secret vsphere-config-secret -n kube-system -o yaml

apiVersion: v1
data:
  csi-vsphere.conf: xxxxxxxxxxxxxxxx
kind: Secret
metadata:
  creationTimestamp: "2020-10-15T21:27:02Z"
  name: vsphere-config-secret
  namespace: kube-system
  resourceVersion: "4634442"
  selfLink: /api/v1/namespaces/kube-system/secrets/vsphere-config-secret
  uid: 1dd8548-91e1-4223-b231-78cefb3c9041
type: Opaque
```

## Storage class

Velero Plugin for vSphere v1.1.0 or higher requires a storage class to restore a PVC. This is because a PVC will be provisioned dynamically at the restore time. If a storage class is not available, restore will fail. For statically provisioned volumes, user can specify `com.vmware.cnsdp.emptystorageclass` as the old storage class name to map to a new existing storage class name at restore time.

## Backup and restore in maintenance mode

Backup or Restore operation while the ESXi host (which has PVC/PV/Pods associated with the workload) in maintenance mode is not recommended as the volumes may be inaccessible during this period.

## Resolve `no space left on device` Issue in Velero Pod

The default capacity of local ephemeral storage in Pods in vSphere with Kubernetes supervisor cluster is set to around
256 MB, which is comparatively too small. With the default configuration, Velero pod might be likely to crash due to
the `no space left on device` issue. To temporarily resolve this issue, users can refer to
[this Kubernetes document](https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/#setting-requests-and-limits-for-local-ephemeral-storage)
and set requests and limits for local ephemeral storage accordingly.

## Resolve `Too Many Requests` Issue in Pulling Images
Currently, container images of Velero and Velero Plugin for vSphere are hosted in Docker Hub.
Due to [the recent rate limiting mechanism](https://www.docker.com/increase-rate-limits) in Docker Hub, you may see
more `ImagePullBackOff` errors with `Too Many Requests` error message while deploying Velero
and Velero Plugin for vSphere. To workaround this issue, please download these container images from Docker Hub,
upload them to the alternative container registry service of your choice, and update image fields in API objects
of Velero and Velero Plugin for vSphere.
