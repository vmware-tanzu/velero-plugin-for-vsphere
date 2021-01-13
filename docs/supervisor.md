# Velero Plugin for vSphere in vSphere with Tanzu Supervisor Cluster

## Table of Contents

1. [Compatibility](#compatibility)
2. [Prerequisites](#prerequisites)
3. [Install](#install)
4. [Uninstall](#uninstall)
5. [Backup](#backup)
6. [Restore](#restore)

## Compatibility

| vSphere Version |                           vSphere CSI Version                          | Kubernetes Version | Velero Version | Velero Plugin for vSphere Version |
|:---------------:|:----------------------------------------------------------------------:|:------------------:|:--------------:|:---------------------------------:|
|  vSphere 7.0 U1c/P02 + ESXi 7.0 U1c/P02 | Bundled with vSphere |     Bundled with vSphere (v1.16-v1.19)    |     v1.5.1     |         v1.1.0 and higher         |

## Prerequisites

* [Install Data Manager](supervisor-datamgr.md)
* The following vSphere privileges are required if users do not have an Administrator role in vSphere.
  * SupervisorServices.Manage
  * Namespaces.Manage
  * Namespaces.Configure

## Install

In a **vSphere with Tanzu** Supervisor cluster, users are supposed to leverage `Velero vSphere Operator` to install velero as well as velero-plugin-for-vsphere, since vSphere users who install velero don't have cluster-admin role in Supervisor cluster. Please refer to
[Installing Velero on Supervisor cluster](velero-vsphere-operator-user-manual.md#installing-velero-on-supervisor-cluster)
for the detail.

**Note**: `Velero vSphere Operator` CLI that comes with `Velero vSphere Operator` aims to provide a similar user experience as the Velero CLI in install and uninstall operations. For other Velero operations, users must continue to use the Velero CLI.

## Uninstall

In a vSphere with Tanzu Supervisor cluster, users should use `Velero vSphere Operator` CLI to uninstall [Uninstalling Velero on Supervisor cluster](velero-vsphere-operator-user-manual.md#uninstalling-velero-on-supervisor-cluster).

**Note**: Disabling the `Velero vSphere Operator` Supervisor Service will also uninstall Velero as well as the ```velero-plugin-for-vsphere``` in the Supervisor cluster. However, as a best practice, it is recommended to uninstall velero and velero-plugin-for-vsphere using `Velero vSphere Operator CLI`.

## vSphere with Tanzu notes

Please read this section before doing backup and restore in vSphere with Tanzu Supervisor Cluster. The details are documented at [vSphere with Tanzu notes](supervisor-notes.md).

## Backup

The backup workflow in a vSphere with Tanzu Supervisor cluster is the same as that in Vanilla Kubernetes cluster. Please refer to [Backup vSphere CNS Block Volumes](vanilla.md#backup-vsphere-cns-block-volumes) in Vanilla Kubernetes Cluster document.

## Restore

The restore workflow in a vSphere with Tanzu Supervisor cluster is the same as that in Vanilla Kubernetes cluster. Please refer to [Restore](vanilla.md#restore) in Vanilla Kubernetes Cluster document.
