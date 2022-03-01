# Velero Plugin for vSphere

![Velero Plugin For vSphere CICD Pipeline](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/workflows/Velero%20Plugin%20For%20vSphere%20CICD%20Pipeline/badge.svg)

## Overview

This repository contains the Velero Plugin for vSphere.  This plugin provides crash-consistent snapshots of vSphere block volumes and backup of volume data into S3 compatible storage.

## Releases

For releases, please refer to the [releases](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/releases) page.

## Kubernetes distributions

**Velero Plugin for vSphere** supports the following Kubernetes distributions:

- [Vanilla Kubernetes](https://github.com/kubernetes/kubernetes)
- [vSphere with Tanzu](https://blogs.vmware.com/vsphere/2019/08/introducing-project-pacific.html)
- [Tanzu Kubernetes Grid Service/TKGS](https://blogs.vmware.com/vsphere/2020/03/vsphere-7-tanzu-kubernetes-clusters.html)

For more information on **vSphere with Tanzu**, (formerly known as **vSphere with Kubernetes** and **Project Pacific**), and especially for information about the role of the Supervisor Cluster, please see [vSphere with Tanzu Configuration and Management](https://docs.vmware.com/en/VMware-vSphere/7.0/vmware-vsphere-with-kubernetes/GUID-152BE7D2-E227-4DAA-B527-557B564D9718.html).

For more information on TKGS, the ```Tanzu Kubernetes Grid Service```, please see [Provisioning and Managing Tanzu Kubernetes Clusters Using the Tanzu Kubernetes Grid Service](https://docs.vmware.com/en/VMware-vSphere/7.0/vmware-vsphere-with-kubernetes/GUID-7E00E7C2-D1A1-4F7D-9110-620F30C02547.html).

## Important note on vSphere with Tanzu networking requirements

**vSphere with Tanzu** supports two distinct networking deployments with the release of vSphere 7.0 Update 1 (U1). The first deployment leverages ```NSX-T``` to provide load balancing services as well as overlay networking for Pod VM to Pod VM communication in the Supervisor Cluster. The second networking configuration supported by vSphere with Tanzu uses native vSphere network distributed switches and a ```HA Proxy``` to provide load balancing services. Native vSphere networking does not provide any overlay networking capabilities, and thus this deployment does not currently support Pod VMs in the Supervisor. Since Pod VMs are a requirement when wish to use the **Velero Plugin for vSphere** in vSphere with Tanzu, it currently requires a networking deployment that uses NSX-T. vSphere with Tanzu deployments that use native vSphere distributed switch networking and the HA Proxy for load balancing cannot currently be backed up using the Velero Plugin for vSphere.

**TKGS** is the Tanzu Kubernetes Grid Service, a service available in vSphere with Tanzu to enable the deployment of TKG (guest) clusters in vSphere with Tanzu namespaces. Whilst TKGS is available in both deployments types of vSphere with Tanzu (NSX-T and native vSphere networking), the ability to backup TKGS guest clusters also requires the Supervisor Cluster to have the Velero Plugin for vSphere to be installed in the Supervisor Cluster. Since this is not possible with vSphere with Tanzu which uses native vSphere distributed switch networking, the Velero Plugin for vSphere is currently unable to backup and restore TKG guest clustes in these deployments.

## Architecture

Velero handles Kubernetes metadata during backup and retore. Velero relies on its plugin to backup and retore PVCs. Velero Plugin for vSphere includes the following components:
* Velero vSphere Operator - a Supervisor Service that helps users install Velero and its vSphere plugin on the Supervisor Cluster; must be enabled through vSphere UI to support backup and restore on Supervisor and Guest Clusters
* vSphere Plugin - deployed with Velero; called by Velero to backup and restore a PVC
* Backupdriver - handles the backup and restore of PVCs; relies on the Data Mover to upload or download data
* Data Mover - handles upload of snapshot data to object store and download of snapshot data from object store

![Velero Plugin for vSphere Architecture](docs/vsphere-plugin-architecture.png)

Backup and restore workflows are described in details [here](docs/backup-restore-workflows.md).

## Velero Plugin for vSphere Installation and Configuration details

For details on how to use Velero Plugin for vSphere for each Kubernetes flavor, refer to the following documents:

- [Guide for Vanilla Kubernetes](docs/vanilla.md)
- [Guide for vSphere with Tanzu](docs/supervisor.md)
- [Guide for Tanzu Kubernetes Grid Service](docs/guest.md)

Note: Velero needs to be installed in each TKG workload cluster.

## Known issues

Known issues are documented [here](docs/known_issues.md).

## Troubleshooting

If you encounter issues, review the [troubleshooting](docs/troubleshooting.md) docs or [file an issue](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/issues).

## Developer Guide

Developer Guide is available at [here](BUILDING.md).
