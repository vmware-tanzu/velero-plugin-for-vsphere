# Data manager configuration for vSphere with Tanzu

The Data Manager is a component of the **Velero plugin for vSphere** responsible for moving the volume snapshot data. It moves the volume snapshot data from the vSphere volume to the remote durable S3 storage on backup, and from remote s3 storage to a vSphere volume during restore.

In a vSphere with Tanzu environment, the Data Manager should be installed as a Virtual Machine on vSphere. It does not come up as container, as it does in a kubernetes vanilla vSphere environment. This is due to networking limitation in accessing vCenter/management network from TKGS clusters or PodVMs.

This document discusses the installation procedure for backup data manager for **Velero plugin for vSphere** with kubernetes.

## Changes with release 1.1.0

Support of vSphere with Tanzu (Supervisor cluster and TKGS cluster) is being added with the release 1.1.0 release.

- As a best practice, Data Manager VMs should be installed on the vSphere compute cluster where the workload cluster is installed.
- Each Data Manager VM can serve upload/download tasks from a single workload cluster and the TKGS clusters in it.
- The Data Manager VMs can be scaled up/down with multiple VMs supporting one workload cluster.

## Setting up Backup Network

It is recommended that the Kubernetes backup/restore traffic be separated from the vSphere management network on a workload cluster. A backup network can be configured as an NSX-T network or traditional TDS network. We can add a VMkernel NIC on each ESXi host in the cluster and set the ```vSphereBackupNFC``` on that NIC. This enables backup network traffic to be sent through that NIC. If the ```vSphereBackupNFC``` is not enabled on the VMkernel NIC, the backup traffic will be sent on the management network.

More details can be found in the [vSphere documentation](https://docs.vmware.com/en/VMware-vSphere/8.0/vsphere-networking/GUID-7BC73116-C4A6-411D-8A32-AD5B7A3D5493.html).

Here are some ways to setup a Backup Network with ```vSphereBackupNFC``` tag enabled on a NSX setup.

If using the existing vSphere Distributed Switch for the cluster (including VCF environments):

1. Go to vCenter Menu -> Networking
2. On the vSphere cluster Distributed Switch, create a DistributedPortGroup called **BackupNetwork**
3. Add VMkernel adapter to the DistributedPortGroup BackupNetwork, attach all the ESXi hosts in the cluster and enable the ```vSphereBackupNFC``` tag

If there is a free physical network adpter on each of the cluster nodes:

1. On vCenter UI - go to ESXi server -> Configure -> Networking -> Virtual switches -> Add Networking
2. Create a new Standard switch on the free network adapter on each ESXi host in the cluster, with a label BackupNetwork
3. Add VMkernel port to each Standard switch with ```vSphereBackupNFC``` tag

## Data Manager Virtual Machine install

The Data Manager Virtual Machine ova can be downloaded from [here](https://vsphere-velero-datamgr.s3-us-west-1.amazonaws.com/datamgr-ob-17253392-photon-3-release-1.1.ova).

It is recommended to power on the Data manager VM after enabling the velero-vsphere service and installing Velero + vSphere plugin in the Supervisor cluster.

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
- Check Velero Data Manager status and logs
  - "docker ps" to confirm data manager container is running.
  - "docker logs velero-datamgr" to get container logs
- Setup DNS settings if required (Might be required in VCF environment)
  - Update DNS and domain in /etc/systemd/network/99-dhcp-en.network
  - restart services
    - systemctl restart systemd-networkd
    - systemctl restart systemd-resolved
  - check /etc/resolv.conf
  - Try restarting data manager container by restarting the VM.
  
If you see that the Data Manager container image is not up, please make sure Velero and the vSphere plugin are installed in the Supervisor cluster. Power off the Data Manager VM and check the VM input/configuration parameters. Then restart the Data Manager VM.
