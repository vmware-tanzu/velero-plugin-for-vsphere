# CSI migrated volume support

Velero Plugin for vSphere v1.4.0 and above supports backing up of migrated vSphere CSI volumes. A migrated vSphere CSI volume is originally provisioned by VMware vSphere Cloud Provider but migrated to vSphere CSI driver. 

To enable this feature, we need to turn on feature gate csi-migrated-volume-support
Below is the default configMap values.
```yaml
kubectl -n velero describe  configmap/velero-vsphere-plugin-feature-states
Name:         velero-vsphere-plugin-feature-states
Namespace:    velero
Labels:       <none>
Labels:       <none>
Annotations:  <none>

  Data
  ====
csi-migrated-volume-support:
  ----
  false
decouple-vsphere-csi-driver:
  ----
  true
local-mode:
```

We can turn it on with below cmd
```yaml
cat velero-vsphere-plugin-feature-states.yaml

apiVersion: v1
data:
   csi-migrated-volume-support: "true"
   decouple-vsphere-csi-driver: "true"
   local-mode: "false"
kind: ConfigMap
metadata:
   name: velero-vsphere-plugin-feature-states

kubectl -n velero apply -f velero-vsphere-plugin-feature-states.yaml
```
## Prerequisite
We need to make sure vSphere CSI migration is properly enabled.
[Migrating In-Tree vSphere Volumes to vSphere Container Storage Plug-in](https://docs.vmware.com/en/VMware-vSphere-Container-Storage-Plug-in/2.0/vmware-vsphere-csp-getting-started/GUID-968D421F-D464-4E22-8127-6CB9FF54423F.html)

Below cmd will print all CRs, 1 for each migrated vSphere CSI volume 
```yaml
kubectl get cnsvspherevolumemigrations.cns.vmware.com -A
```
