# Known Issues


V1.0.2



## v1.0.1

2. The restore might place the restored PVs in a vSphere datastore which is imcompatible with the StorageClass
specified in the restored PVC, as we don't rely on the StorageClass for the PV placement on restore in the release v1.0.0.
A fix to the PV placement based on the StorageClass can be expected in a future release.

## v1.0.0
1. Restore fails if the K8s node VMs are placed in any sub-folders of vSphere VM inventory.
    1. Workaround: move node VMs to the top level of VM inventory. (for users of release v1.0.0)
    2. Solution: the issue was resolved in the master branch at commit 1c3dd7e20c3198e4a94a5a17874d4c474b48c2e2.
2. The restore might place the restored PVs in a vSphere datastore which is imcompatible with the StorageClass
specified in the restored PVC, as we don't rely on the StorageClass for the PV placement on restore in the release v1.0.0.
A fix to the PV placement based on the StorageClass can be expected in a future release.
3. The plugin v1.0.0 has issues with some variants of Kubernetes on vSphere.
    1. In a TKG cluster, the vSphere CSI credential secret, i.e., VC credential,
    is named  `csi-vsphere-config` in the `kube-system` namespace, which is different than the expected
    `vsphere-config-secret` in the `kube-system` namespace. As a workaround, add another secret, `vsphere-config-secret`, 
    with VC credential available in the secret `csi-vsphere-config`. Some more tips on using this workaround:
    [Issue #63](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/issues/63).
        1. Make sure to use quotes to wrap around the value of each key-value pair in the `csi-vsphere.conf`
        when constructing `vsphere-config-secret`.
        2. Make sure to add one more key-value pair, `port = "443"`, under the `VirtualCenter` section.
        As far as we know, the `port` key is absent from the `csi-vsphere.conf` of the secret `csi-vsphere-config` in TKG.

