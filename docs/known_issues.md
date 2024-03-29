# Velero Plugin for vSphere Known Issues

This section lists the major known issues with Velero Plugin for vSphere. For a complete list of issues, check the [Github issues](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/issues) page. If you notice an issue not listed in Github issues page, please file an issue on the Github repository.

## vSphere CSI Driver Config File

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

## Storage Class

Velero Plugin for vSphere v1.1.0 or higher requires a storage class to restore a PVC. This is because a PVC will be provisioned dynamically at the restore time. If a storage class is not available, restore will fail. To restore a PVC which is backed up without a storage class, i.e., statically provisioned persistent volumes on Vanilla cluster, user can specify `com.vmware.cnsdp.emptystorageclass` as the old storage class name to map to a new existing storage class name at restore time. See document about [storage class mapping](storageclass-mapping.md) for more details.

## Backup and Restore in Maintenance Mode

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

## Backup Statically Provisioned Persistent Volumes on Supervisor Cluster

On Supervisor Cluster, persistent volumes can be statically provisioned using `CNSRegisterVolume`. This custom resource, `CNSRegisterVolume`, however, cannot be included in the backup. See details [here](supervisor-notes.md). To workaround this limitation, this custom resource should be excluded from the backup.

```
velero backup create my-backup --include-namespaces my-namespace --snapshot-volumes --exclude-resources cnsregistervolumes.cns.vmware.com
```

## Provide Registry Credentials for Images

Users might require ways to provide registry credentials when deploying velero and vSphere plugin with images in private registries. Below are solutions in different cases.
* In Vanilla cluster or Tanzu Kubernetes Grid Service cluster, users can [add the image pull secrets to the corresponding service accounts](https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/#add-image-pull-secret-to-service-account).
* In vSphere with Tanzu cluster, users can use the `--use-private-registry` option available in [velero vSphere operator CLI](velero-vsphere-operator-cli.md) to indicate pulling image from private registry. As a prerequisite, users are expected to provide the registry credential when enabling `Velero vSphere Operator` Supervisor Service.

## Volume Expansion Not Supported when there are Snapshots

If upload fails during a backup, the local snapshot will not be deleted. Velero vSphere Plugin will keep retrying the upload. If the user wants to expand volume, it will fail because volume expansion is not supported when there are snapshots on the volume.

## Backup and Restore with vSphere with IPV6

Users may hit the following error `too many colons in address` if they specify VC IP in IPV6 format in `csi-vsphere.conf` file and create `velero-vsphere-config-secret` from this file when deploying Velero Plugin for vSphere. 

```
time="2023-11-06T10:53:21Z" level=error msg="Failed at copying to remote repository" Local PEID="ivd:fdc85e45-7c61-453f-9487-b7e47d020cd9:a6183167-d52d-4a83-932f-77104fac93e3" error="dial tcp: address 2620:124:6020:c304:0:a:0:763:443: too many colons in address" logSource="/go/src/github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/dataMover/data_mover.go:120"
```
The workaround to this problem is to use VC FQDN instead of IPV6 format in `csi-vsphere.conf` file. See the following example.

```
[Global]
cluster-id = "cluster0"

[VirtualCenter "vCenter-FQDN"]
insecure-flag = "true"
user = "user@vsphere.local"
password = "password"
port = "443"
datacenters = "dc0"
```
