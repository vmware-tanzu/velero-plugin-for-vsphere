# Data manager configuration for vSphere with Kubernetes
The Data Manager is a component of the velero vSphere plugin responsible for moving the volume snapshot data. It moves
the volume snapshot data from the vSphere volume to the remote durable S3 storage on backup, and from remote s3 storage
to a vSphere volume during restore.

In a vSphere with Kubernetes environment, the data manager should be installed as a Virtual Machine on vSphere. It does
not come up as container, as it does in a kubernetes vanilla vSphere environment. This is due to networking limitation in
accessing vCenter/management network from TKGS clusters or PodVMs.

This document discusses the installation procedure for backup data manager for velero plugin for vSphere with kubernetes.

## Changes with release 1.1.0
Support of vSphere with kubernetes (supervisor/workload cluster and TKGS cluster) is being added with the release 1.1.0 release.

- As a best practice, Data Manager VMs should be installed on the vSphere compute cluster where the workload cluster is installed.
- Each Data Manager VM can serve upload/download tasks from a single workload cluster and the TKGS clusters in it.
- The Data Manager VMs can be scaled up/down with multiple VMs supporting one workload cluster. 

## Setting up Backup Network
It is recommended that the kubernetes backup/restore traffic be separated from the vSphere management network on a
workload cluster. A backup network can be configured as an NSX network or traditional TDS network. We can add a VMkernel
NIC on each host in the cluster and set the vSphereBackupNFC on that NIC. This enables backup network traffic to be sent
through that NIC. If the vSphereBackupNFC is not enabled, the backup traffic will be sent on the management network.

More details can be found in the [vSphere documentation](https://code.vmware.com/docs/12628/virtual-disk-development-kit-programming-guide/GUID-5D166ED1-7205-4110-8D72-0C51BB63CC3D.html).

Here are some ways to setup a Backup Network with vSphereBackupNFC tag enabled on a NSX setup.

If using the existing vSphere Distributed Switch for the cluster (Also for VCF environment):
1. Go to vCenter Menu -> Networking
2. On the cluster Distributed switch, create a DistributedPortGroup called BackupNetwork
3. Add VMkernel adapter to the DistributedPortGroup BackupNetwork, attach all the ESXi hosts in the cluster and 
enable the "vSphereBackupNFC" tag

If there is a free physical network adpter on each of the cluster nodes
1. On vCenter UI - go to ESXi server -> Configure -> Networking -> Virtual switches -> Add Networking
2. Create a new Standard switch on the free network adapter on each ESXi host in the cluster, with a label BackupNetwork
3. Add VMkernel port to each Standard switch with vSphereBackupNFC tag

## Data Manager Virtual Machine install
The Data Manager Virtual Machine ova can be downloaded from [here](https://vsphere-velero-datamgr.s3-us-west-1.amazonaws.com/datamgr-ob-17253392-photon-3-release-1.1.ova).

It is recommended to power on the Data manager VM after enabling the velero-vsphere service and installing velero + vSphere
plugin in the workload cluster.

1. Install the DataManager VM in the vSphere cluster on the backup network. Customize any configuration based on your environment.
2. Configure the input parameters for the VM. On the VC UI go to VM -> Edit Settings -> VM Options -> Advanced -> Edit Configuration Parameters
   - guestinfo.cnsdp.wcpControlPlaneIP
   - guestinfo.cnsdp.vcAddress
   - guestinfo.cnsdp.vcUser
   - guestinfo.cnsdp.vcPassword
   - guestinfo.cnsdp.vcPort
   - guestinfo.cnsdp.veleroNamespace
   - guestinfo.cnsdp.datamgrImage (if not configured, will use the image from dockerhub vsphereveleroplugin/data-manager-for-plugin:1.1.0)
   - guestinfo.cnsdp.updateKubectl (default false, to avoid kubectl from wcp master on every VM restart)
3. Power On the Data Manager VM

## Debugging Data Manager issues, container image
- Login to Data Manager VM
  - Power on the VM and login - ssh or web console
  - First time root password is "changeme". Update it.
- Check velero data manager status and logs
  - "docker ps" to confirm data manager container is running.
  - "docker logs velero-datamgr" to get container logs
- Setup DNS settings if required (Might be required in VCF environment)
  - Update DNS and domain in /etc/systemd/network/99-dhcp-en.network
  - restart services
     - systemctl restart systemd-networkd
     - systemctl restart systemd-resolved
  - check /etc/resolv.conf
  - Try restarting data manager container by restarting the VM.
  
If you see that the Data Manager container image is not up, please make sure velero and vsphere plugin are installed in
the workload cluster, power off the VM, check the VM input/configuration parameters and restart the Data Manager.
