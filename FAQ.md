# Frequently Asked Questions

## v1.0.0
1. The restore would fail if the K8s node VMs are placed in any sub-folders of vSphere VM inventory.
    1. Workaround: moving node VMs to the top level of VM inventory. (for users of the release v1.0.0)
    2. Solution: the issue was resolved post the release v1.0.0 in the master branch at commit 1c3dd7e20c3198e4a94a5a17874d4c474b48c2e2.
2. The restore might possibly place the restored PVs in a vSphere datastore which is imcompatible with the StorageClass
specified in the restored PVC, as we don't rely on the StorageClass for the PV placement on restore in the release v1.0.0.
A fix to the PV placement based on the StorageClass can be expected in the next release.
3. The plugin v1.0.0 is supposed to work well with Vanilla K8s cluster. We have not yet officially
supported other type of K8s clusters, so there might be some expected issues.
    1. In a TKG cluster, the secret of vSphere CSI credential, i.e., VC credential,
    is named as `csi-vsphere-config` in the `kube-system` namespace, which might be problematic when using velero
    with velero-plugin-for-vsphere v1.0.0 since it always tries to retrive the VC credential from the
    serect `vsphere-config-secret` in the `kube-system` namespace. Hence, to use the plugin v1.0.0 in the TKG cluster,
    the workaround is to add one another secret, `vsphere-config-secret`, with VC credential available in the secret
    `csi-vsphere-config`. Below are a few of tips about the workaround based on the observation in
    [Issue #63](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/issues/63).
        1. Make sure to use quotes to wrap around the value of each key-value pair in the `csi-vsphere.conf`
        when constructing `vsphere-config-secret`.
        2. Make sure to add one more key-value pair, `port = "443"`, under the `VirtualCenter` section.
        As far as we know, the `port` key is absent from the `csi-vsphere.conf` of the secret `csi-vsphere-config` in TKG.
