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

## Backup and restore in maintenance mode

Backup or Restore operation while the ESXi host (which has PVC/PV/Pods associated with the workload) in maintenance mode is not recommended as the volumes may be inaccessible during this period.
