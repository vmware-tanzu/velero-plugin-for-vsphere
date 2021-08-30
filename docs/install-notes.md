# Install Notes with Customized Images

## TL;DR

- If you plan to install Velero Plugin for vSphere with released images, skip this doc.
- If you plan to install Velero Plugin for vSphere with customized images, follow the steps below. For more detail, please read through the rest of the doc.
    1. Download the released images of `velero-plugin-for-vsphere`, `backup-driver` and `data-manager-for-plugin`.
    2. Rename(i.e. docker tag) them with matching `<Registry endpoint and path>` and `<Version tag>` and upload it the customized repositories.
    3. Install with the customized `velero-plugin-for-vsphere` image. (If unexpected `backup-driver` and `data-manager-for-plugin` images is detected to be used, check out if [the fall-back cases](#fall-back-in-image-parsing) apply)

## Background

When installing `velero-plugin-for-vsphere` in Vanilla cluster, it further deploys another two components, a `backup-driver` Deployment and a `data-manager-for-plugin` Daemonset, behind the scene. Specifically, in Supervisor and Guest clusters, only a `backup-driver` deployment is deployed additionally. With the container image of `velero-plugin-for-vsphere` provided by Velero users, the matching `backup-driver` and `data-manager-for-plugin` images will be parsed based on the following mechanism.

## Image Parsing Mechanism

Container images can be formalized into the following pattern.

```
<Registry endpoint and path>/<Container name>:<Version tag>
```

Given any `velero-plugin-for-vsphere` container image from user input, the corresponding images of `backup-driver` and `data-manager-for-plugin` with matching `<Registry endpoint and path>` and `<Version tag>` will be parsed.

For example, given the `velero-plugin-for-vsphere` container image as below,

```
abc.io:8989/x/y/.../z/velero-plugin-for-vsphere:vX.Y.Z
``` 

The following **matching** images of `backup-driver` and `data-manager-for-plugin` are expected to be pulled. 

```
abc.io:8989/x/y/.../z/backup-driver:vX.Y.Z
abc.io:8989/x/y/.../z/data-manager-for-plugin:vX.Y.Z
``` 

### Customized Images

Except using the [released container images](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/releases), customized images are also supported when installing `velero-plugin-for-vsphere`. However, users need to make sure that the matching images of `backup-driver` and `data-manager-for-plugin` of customized images are available in the expected registry and are accessible from the Kubernetes cluster. For example, in the air-gapped environment, customized images from private registry are expected since the released images in docker hub are not accessible.

### Fall-back in Image Parsing

If there are any issues or errors in parsing the matching images of `backup-driver` and `data-manager-for-plugin`, it would fall back to corresponding images from the official `velerovsphereplugin` repositories in docker hub. The following issues would trigger the fall-back mechanism.

1. Unexpected container name is used in the customized `velero-plugin-for-vsphere` image from user input. For example, `x/y/velero-velero-plugin-for-vsphere:v1.1.1` is used.
2. Velero deployment name is customized to be anything other than `velero`. For example, Velero deployment name is updated to `velero-server` in Velero manifests file before deploying Velero. However, the existing image parsing mechanism in `velero-plugin-for-vsphere` can only recognize the Velero deployment with the fixed name, `velero`.
