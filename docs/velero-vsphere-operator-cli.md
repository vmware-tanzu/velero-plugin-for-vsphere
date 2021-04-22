# Velero vSphere Operator CLI Reference

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Install Velero & Plugins](#install-velero--plugins)
3. [Uninstall Velero & Plugins](#uninstall-velero--plugins)
4. [Configure Velero & Plugins](#configure-velero--plugins)

## Prerequisites

* `Velero vSphere Operator` supervisor service is expected to be **enabled** before running any CLI command.
* download `vsphere-velero` cli from [velero-plugin-for-vsphere release v1.1.0]('releases/download/v1.1.0/velero-vsphere-1.1.0-linux-amd64.tar.gz')

## Install Velero & Plugins

1. [Install Usage](#command-usage)
2. [Install Prerequisites](#install-prerequisites)
3. [Install Notes](#install-notes)
4. [Install Examples](#install-examples)
5. [Check Install Status](#check-install-status)

### Install Usage

```bash
Install Velero Instance

Usage:
  velero-vsphere install [flags]

Flags:
      --backup-location-config mapStringString     configuration to use for the backup storage location. Format is key1=value1,key2=value2
      --bucket string                              name of the object storage bucket where backups should be stored
  -h, --help                                       help for install
      --image string                               image to use for the Velero server pods. Optional. (default "velero/velero:v1.5.1")
  -n, --namespace string                           the namespace where to install Velero. Optional. (default "velero")
      --no-default-backup-location                 flag indicating if a default backup location should be created. Must be used as confirmation if --bucket or --provider are not provided. Optional.
      --no-secret                                  flag indicating if a secret should be created. Must be used as confirmation if --secret-file is not provided. Optional.
      --plugins stringArray                        plugin container images to install into the Velero Deployment
      --provider string                            provider name for backup and volume storage
      --secret-file string                         file containing credentials for backup and volume provider. If not specified, --no-secret must be used for confirmation. Optional.
      --snapshot-location-config mapStringString   configuration to use for the volume snapshot location. Format is key1=value1,key2=value2
      --upgrade-option string                      upgrade option: manual or automatic. Optional. (default "Manual")
      --use-private-registry                       whether or not to pull instance images from a private registry. Optional
      --use-volume-snapshots                       whether or not to create snapshot location automatically. Optional (default true)
      --version string                             version for velero to be installed. Optional. (default "v1.5.1")

Global Flags:
      --enable-leader-election   Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.
      --kubeconfig string        Paths to a kubeconfig. Only required if out-of-cluster.
      --master --kubeconfig      (Deprecated: switch to --kubeconfig) The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.
      --webhook-port int         Webhook server port (set to 0 to disable)
```

### Install Prerequisites

* Users are expected to create a Supervisor namespace via vSphere UI/API/DCLI before running the install command. Otherwise, the install command would fail.
* Users are expected to ensure that the Cluster Config Status in the Workload Management plane shows `Running` before running the install command. Otherwise, the Velero pod will be stuck at `Pending` state and the install operation can never be completed. There are multiple ways to check the status.
  * **UI**. Select `Workload Management` from `menu` in the home page of vSphere UI. Then, navigate to the `Clusters` tab.
  * **DCLI**. Run the following command and check the `config_status` field for the corresponding cluster.

    ```bash
    dcli com vmware vcenter namespacemanagement clusters list
    ```

### Install Notes

* The Velero vSphere plugin, velero-plugin-for-vsphere, must be provided in the install command. Otherwise, the install command would fail.

### Install Examples

Below are some examples.

1. Installing Velero with default backup location and snapshot location

    ```bash
    velero-vsphere install \
           --namespace velero \
           --version v1.5.1 \
           --provider aws \
           --plugins velero/velero-plugin-for-aws:v1.1.0,vsphereveleroplugin/velero-plugin-for-vsphere:1.1.0 \
           --bucket $BUCKET \
           --secret-file ~/.aws/credentials \
           --snapshot-location-config region=$REGION \
           --backup-location-config region=$REGION
    ```

2. Installing Velero without default backup location and snapshot location

    ```bash
    velero-vsphere install \
        --version v1.5.1 \
        --plugins vsphereveleroplugin/velero-plugin-for-vsphere:1.1.0 \
        --no-secret \
        --use-volume-snapshots=false \
        --no-default-backup-location
    ```

3. Install Velero in the an Air-gap environment

    ```bash
    velero-vsphere install \
        --namespace velero \
        --image <private registry name>/velero:v1.5.1 --use-private-registry \
        --provider aws \
        --plugins <private registry name>/velero-plugin-for-aws:v1.1.0,<private registry name>/velero-plugin-for-vsphere:1.1.0 \
        --bucket $BUCKET \
        --secret-file ~/.minio/credentials \
        --snapshot-location-config region=$REGION \
        --backup-location-config region=$REGION,s3ForcePathStyle="true",s3Url=$S3URL
    ```

### Check Install Status

The following command can be used to check if installing Velero and vSphere plugin are completed in Supervisor cluster.

```bash
kubectl -n velero get veleroservice default -o json | jq '.status'
```

Before the install operation is completed, the `installphase` field in the `veleroservice.status` will be left unset.
When the install operation is completed successfully, the follow result will be returned.

```yaml
{
  "enabled": true,
  "installphase": "Completed",
  "version": "v1.5.1"
}
```

Instead, when the install operation is completed with any failure, corresponding error message
will be shown. Below is an example.

```yaml
{
  "enabled": true,
  "installmessage": "Failed to install Velero since there is existing Velero instance in the cluster. Error: The expected annotation already exists, velero-service=velero",
  "installphase": "Failed",
}
```

## Uninstall Velero & Plugins

```bash
Uninstall Velero Instance

Usage:
  velero-vsphere uninstall [flags]

Flags:
  -h, --help               help for uninstall
  -n, --namespace string   The namespace of Velero instance. Optional. (default "velero")
```

Below is an example,

```bash
velero-vsphere uninstall -n velero
```

**Note**: users are expected to delete the corresponding Supervisor namespace via vSphere UI/API/DCLI after
running the command above to uninstall Velero.
