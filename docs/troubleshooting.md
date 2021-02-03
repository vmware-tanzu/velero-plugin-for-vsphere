# Troubleshooting

To troubleshoot issues in velero-plugin-for-vsphere, the following information is expected.

## Environment

The following environment information will also be **required**.

- velero-plugin-for-vsphere version
- Kubernetes cluster flavor
- vSphere CSI driver version
- Velero version (use `velero version`)
- Velero features (use `velero client config get features`)
- Kubernetes version (use `kubectl version`)
- vSphere versions, including vCenter version and ESXi version

## Logs

### General/Vanilla

The output of the following command will be **required**.

- `kubectl -n <velero namespace> logs deploy/velero` - Velero log 
- `velero backup/restore get/describe <name>` - Velero Backup and Restore CRs
- `kubectl -n <velero namespace> logs deploy/backup-driver` - Backup Driver log
- `kubectl -n velero logs pod/<data manager pod name>` - Data Manager log on **each** pod of data-manager-for-plugin daemonset.
- `kubectl cp <namespace>/<data manager pod name>:/tmp/vmware-root <destination dir>` - VDDK log on **each** pod of data-manager-for-plugin daemonset. 
- `kubectl -n <velero namespace> get <plugin crd name> -o yaml` - Plugin Upload and Download CRs
- `kubectl -n <app namespace> get <plugin crd name> -o yaml` - Plugin Backup Driver CRs

Other logs might also be requested if necessary.

* vSphere log bundles, including vCenter log bundle and ESXi log bundles.
* vSphere CSI driver log (please refer to [vSphere CSI driver troubleshooting](https://vsphere-csi-driver.sigs.k8s.io/troubleshooting.html))
* Kubernetes logs for the following components, kube-controller-manager, kubelet, kube-scheduler, kube-api-server

### Project Pacific

Apart from logs in the general case, extra logs might also be optionally expected depends on the cluster flavor.

#### Tanzu Kubernetes Grid Service

- Tanzu Kubernetes Cluster support bundle. Please refer to [Troubleshooting Tanzu Kubernetes Clusters](https://docs.vmware.com/en/VMware-vSphere/7.0/vmware-vsphere-with-tanzu/GUID-0BAEA3D2-23AF-477B-8948-6A7D87CD7F62.html) for more detail.

#### vSphere with Tanzu

- `kubectl -n <velero vsphere operator namespace> logs deploy/velero-app-operator` - Velero vSphere Operator log
- `kubectl -n vmware-system-appplatform-operator-system logs sts/vmware-system-appplatform-operator-mgr` - App Platform Operator log
- `VC UI Menu -> Workload Management -> Clusters -> Export Logs with expected cluster selected` - Workload Management/WCP log bundle

