# Velero Plugin for vSphere 

![Velero Plugin For vSphere CICD Pipeline](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/workflows/Velero%20Plugin%20For%20vSphere%20CICD%20Pipeline/badge.svg)

## Overview
This repository contains the Velero Plugin for vSphere.  This plugin provides crash-consistent snapshots of vSphere block volumes and backup of volume data into S3 compatible storage.

## Compatibility
* Velero - Version 1.5.1 or above
* vSphere - Version (TBD)  or above
* vSphere CSI/CNS driver 2.0.0 or above
* Kubernetes 1.17 or above

### Kubernetes distributions

Velero Plugin for vSphere supports the following Kubernetes distributions in the upcoming release:

- [Vanilla Kubernetes](https://github.com/kubernetes/kubernetes)
- [vSphere with Kubernetes](https://blogs.vmware.com/vsphere/2019/08/introducing-project-pacific.html) aka Supervisor Cluster. For more information, see [vSphere with Kubernetes Configuration and Management](https://docs.vmware.com/en/VMware-vSphere/7.0/vmware-vsphere-with-kubernetes/GUID-152BE7D2-E227-4DAA-B527-557B564D9718.html).
- [Tanzu Kubernetes Grid Service](https://blogs.vmware.com/vsphere/2020/03/vsphere-7-tanzu-kubernetes-clusters.html). For more information, see [Provisioning and Managing Tanzu Kubernetes Clusters Using the Tanzu Kubernetes Grid Service](https://docs.vmware.com/en/VMware-vSphere/7.0/vmware-vsphere-with-kubernetes/GUID-7E00E7C2-D1A1-4F7D-9110-620F30C02547.html).

For details on how to use Velero Plugin for vSphere for each Kubernetes flavor, refer to the following docs:

- [Vanilla Kubernetes](docs/vanilla.md)
- [vSphere with Kubernetes](docs/supervisor.md)
- [Tanzu Kubernetes Grid Service](docs/guest.md)

### Latest release:
For 1.0.2 release, see documenation [here](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/tree/v1.0.2)
