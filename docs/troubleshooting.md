# Troubleshooting

To troubleshoot issues in velero-plugin-for-vsphere, the following information is expected.

## Environment

The following environment information will also be **required**.

- Velero version (use `velero version`)
- Velero features (use `velero client config get features`)
- velero-plugin-for-vsphere version, and all other plugins in velero deployment
- Kubernetes cluster flavor (`Vanilla`, `Tanzu Kubernetes Grid Service` or `vSphere with Tanzu`)
- vSphere CSI driver version, and all images  in `vsphere-csi-controller` deployment and `vsphere-csi-node` daemonset
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

### VDDK

Follow the steps below to configure VDDK log level.

* Create a ConfigMap to set VDDK log level
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  # any name can be used; Velero uses the labels (below)
  # to identify it rather than the name
  name: vddk-config
  # must be in the velero namespace
  namespace: velero
  labels:
    # this label identifies the ConfigMap as
    # config for vddk
    # only one vddk ConfigMap should exist at one time
    velero.io/vddk-config: vix-disk-lib
data:
  # NFC LogLevel, default is 1 (0 = Quiet, 1 = Error, 2 = Warning, 3 = Info, 4 = Debug)
  vixDiskLib.nfc.LogLevel: "4"
  # Advanced transport functions log level
  # Default is 3 (0 = Panic(failure only), 1 = Error, 2 = Warning, 3 = Audit, 4 = Info, 5 = Verbose, 6 = Trivia)
  vixDiskLib.transport.LogLevel: "5"
```
* Manually restart data manager pod \
 `kubectl -n velero delete daemonset.apps/datamgr-for-vsphere-plugin` \
 `kubectl delete crds uploads.datamover.cnsdp.vmware.com downloads.datamover.cnsdp.vmware.com` \
 `kubectl -n velero scale deploy/velero --replicas=0` \
 `kubectl -n velero scale deploy/velero --replicas=1`
* Log into data manager pod to check VDDK log file. The default path should be /tmp/vmware-XXX/vixDiskLib-XXX.log. \
 `kubectl exec -n velero -it datamgr-for-vsphere-plugin-XXXXX -- /bin/bash` \
 If `/bin/bash` is not available, use `/bin/sh`:
 `kubectl exec -n velero -it datamgr-for-vsphere-plugin-XXXXX -- /bin/sh` \
 Note that `vixDiskLib-XXX.log` is the VDDK log file that needs to be collected for trouble shooting VDDK issues.
