# Velero Plugin for vSphere in vSphere with Tanzu Supervisor Cluster

## Table of Contents

1. [Compatibility](#compatibility)
2. [Prerequisites](#prerequisites)
3. [Install](#install)
4. [Uninstall](#uninstall)
5. [Backup](#backup)
6. [Restore](#restore)

## Compatibility

| Velero Plugin for vSphere Version | vSphere Version  | Kubernetes Version                                     | vSphere CSI Driver Version | Velero Version | Velero vSphere Operator Version | Data Manager Version | Velero Plugin for AWS | vSphere Plugin Deprecated | vSphere Plugin EOL Date      |
|-----------------------------------|------------------|--------------------------------------------------------|----------------------------|----------------|---------------------------------|---------------|-----------------------|------------|---------------|
| 1.6.0                             | 9.0              | Bundled with vSphere (1.29+)                           | Bundled with vSphere       | 1.13.2          | 1.8.0                           | 1.2.0        | 1.6.1/1.6.2           | No         | N/A    |
| 1.6.0                             | 8.0U3d           | Bundled with vSphere (1.27-1.29)                       | Bundled with vSphere       | 1.13.2          | 1.7.0                           | 1.2.0        | 1.6.1                 | No         | N/A    |
| 1.5.4                             | 8.0U3/8.0U3a     | Bundled with vSphere (1.27-1.29)                       | Bundled with vSphere       | 1.13.1          | 1.6.1                           | 1.2.0        | 1.6.0                 | No         | N/A    |
| 1.5.4                             | 8.0U2d           | Bundled with vSphere (1.25-1.27)                       | Bundled with vSphere       | 1.13.1          | 1.6.1                           | 1.2.0        |                       | No         | N/A           |
| 1.5.3                             | 8.0U3            | Bundled with vSphere (1.26-1.28)                       | Bundled with vSphere       | 1.13.1          | 1.6.1                           | 1.2.0        | 1.6.0                 | No         | N/A    |
| 1.5.3                             | 8.0U2b           | Bundled with vSphere (1.25-1.27)                       | Bundled with vSphere       | 1.13.1          | 1.6.0                           | 1.2.0        |                       | No         | N/A           |
| 1.5.1                             | 8.0P02/8.0U2     | Bundled with vSphere (1.24-1.26)                       | Bundled with vSphere       | 1.11.1          | 1.5.0                           | 1.2.0       |                       | No         | N/A           |
| 1.5.1                             | 8.0U1            | Bundled with vSphere (1.24)                            | Bundled with vSphere       | 1.10.2          | 1.4.0                           | 1.2.0       |                       | No         | N/A           |
| 1.5.1                             | 8.0c             | Bundled with vSphere (1.24)                            | Bundled with vSphere       | 1.10.2          | 1.4.0                           | 1.2.0        |                       | No         | N/A           |
| 1.4.2                             | 8.0              | Bundled with vSphere (1.22-1.23)                       | Bundled with vSphere       | 1.9.2          | 1.3.0                           | 1.2.0      |                       | No         | N/A           |
| 1.6.0                             | 70u3u            | Bundled with vSphere (1.27)                            | Bundled with vSphere       | 1.13.1          | 1.6.1                           | 1.2.0        |                       | No         | N/A           |
| 1.4.0                             | 7.0U3e/f/h       | Bundled with vSphere (1.22)                            | Bundled with vSphere       | 1.8.1          | 1.2.0                           | 1.1.0       |                       | No         | N/A           |
| 1.3.1                             | 7.0U1c/P02 - 7.0U3d | Bundled with vSphere (1.16-1.19, 1.18-1.20, 1.19-1.21) | Bundled with vSphere       | 1.5.1          | 1.1.0                           | 1.1.0       |                       | No         | N/A           |
| 1.3.0                             | 7.0U1c/P02 - 7.0U3d | Bundled with vSphere (1.16-1.19, 1.18-1.20, 1.19-1.21) | Bundled with vSphere       | 1.5.1          | 1.1.0                           | 1.1.0       |                       | Yes        | December 2022 |
| 1.2.1                             | 7.0U1c/P02 - 7.0U2 | Bundled with vSphere (1.16-1.19, 1.18-1.20)            | Bundled with vSphere       | 1.5.1          | 1.1.0                           | 1.1.0        |                       | Yes        | June 2023     |
| 1.2.0                             | 7.0U1c/P02 - 7.0U2 | Bundled with vSphere (1.16-1.19, 1.18-1.20)            | Bundled with vSphere       | 1.5.1          | 1.1.0                           | 1.1.0        |                       | Yes        | December 2022 |
| 1.1.1                             | 7.0U1c/P02       | Bundled with vSphere (1.16-1.19)                       | Bundled with vSphere       | 1.5.1          | 1.1.0                           | 1.1.0       |                       | No         | N/A           |
| 1.1.0                             | 7.0U1c/P02       | Bundled with vSphere (1.16-1.19)                       | Bundled with vSphere       | 1.5.1          | 1.1.0                           | 1.1.0       |                       | Yes        | December 2022 |

## Prerequisites

* [Install Data Manager](supervisor-datamgr.md)
* The following vSphere privileges are required if users do not have an Administrator role in vSphere.
  * SupervisorServices.Manage
  * Namespaces.Manage
  * Namespaces.Configure
* Download `Velero vSphere Operator` CLI [from here](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/releases/download/v1.1.0/velero-vsphere-1.1.0-linux-amd64.tar.gz).

## vSphere with Tanzu notes

Please read [vSphere with Tanzu notes](supervisor-notes.md) before moving forward with the following sections.

## Install

In a **vSphere with Tanzu** Supervisor cluster, users are supposed to leverage `Velero vSphere Operator` to install velero as well as velero-plugin-for-vsphere, since vSphere users who install velero don't have cluster-admin role in Supervisor cluster. Please refer to
[Installing Velero on Supervisor cluster](velero-vsphere-operator-user-manual.md#installing-velero-on-supervisor-cluster)
for the detail.

**Note**: `Velero vSphere Operator` CLI that comes with `Velero vSphere Operator` aims to provide a similar user experience as the Velero CLI in install and uninstall operations. For other Velero operations, users must continue to use the Velero CLI. Please download `Velero vSphere Operator` CLI [from here](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/releases/download/v1.1.0/velero-vsphere-1.1.0-linux-amd64.tar.gz) if you haven't done so.

### Install with self-signed certificate

**Note**: Currently using self-signed certificate to connect to the object store is not supported on supervisor cluster. please refer to [velero-plugin-for-vsphere with a storage provider secured by a self-signed certificate](self-signed-certificate.md).

## Uninstall

In a vSphere with Tanzu Supervisor cluster, users should use `Velero vSphere Operator` CLI to uninstall [Uninstalling Velero on Supervisor cluster](velero-vsphere-operator-user-manual.md#uninstalling-velero-on-supervisor-cluster).

**Note**: Users are expected to uninstall velero and velero-plugin-for-vsphere using `Velero vSphere Operator CLI`.

## Backup

The backup workflow in a vSphere with Tanzu Supervisor cluster is the same as that in Vanilla Kubernetes cluster. Please refer to [Backup vSphere CNS Block Volumes](vanilla.md#backup-vsphere-cns-block-volumes) in Vanilla Kubernetes Cluster document.

## Restore

In a vSphere with Tanzu Supervisor cluster, users need to take extra steps via either vSphere UI or VMware [DCLI](https://code.vmware.com/web/tool/3.0.0/vmware-datacenter-cli) before restoring a workload.

1. Create a namespace in Supervisor cluster.
2. Configure the Storage policy in the namespace.

The rest of restore workflow in a vSphere with Tanzu Supervisor cluster is the same as that in Vanilla Kubernetes cluster. Please refer to [Restore](vanilla.md#restore) in Vanilla Kubernetes Cluster document.
