# Velero Plugin for vSphere

![Velero Plugin For vSphere CICD Pipeline](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/workflows/Velero%20Plugin%20For%20vSphere%20CICD%20Pipeline/badge.svg)

## Overview

This repository contains the Velero Plugin for vSphere.  This plugin provides plugins for velero based on [go-plugin](https://github.com/hashicorp/go-plugin).
The plugins are BackupItemAction and RestoreItemAction to filter resources for backup and restore. Furthermore, the RestoreItemAction also makes some necessary modifications for the VKS resources.

## Releases

For releases, please refer to the [releases](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/releases) page.

## Kubernetes distributions

**Velero Plugin for vSphere** supports the following Kubernetes distributions:

- vSphere Kubernetes Service/VKS

For more information on VKS supervisor, the ```VCF Kubernetes Service supervisor```, please see [Managing vSphere Kubernetes Service with vSphere Supervisor](https://techdocs.broadcom.com/us/en/vmware-cis/vcf/vsphere-supervisor-services-and-standalone-components/latest/managing-vsphere-kubernetes-service.html).

## Known issues

Known issues are documented [here](docs/known_issues.md).

## Troubleshooting

If you encounter issues, review the [troubleshooting](docs/troubleshooting.md) docs or [file an issue](https://github.com/vmware-tanzu/velero-plugin-for-vsphere/issues).

## Developer Guide

Developer Guide is available at [here](BUILDING.md).
