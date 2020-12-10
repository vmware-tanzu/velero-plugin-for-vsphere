# Velero vSphere Change Block Tracking

Change Block Tracking (CBT) configuration is required to support incremental backups of vSphere volumes.

**NOTE**: Currently we only support enabling CBT for block volumes attached to guest clusters.

## Change Block Tracking for guest clusters

Change Block Tracking can be enabled/disabled for guest clusters using the velero vSphere operator [configure](velero-vsphere-operator-cli.md#configure-velero--plugins) command.
Configuring CBT is supported only when velero is installed in the supervisor cluster. The default value is CBT disabled.

In order to support incremental backups, CBT has to be enabled for the virtual machine where the volume is attached and for
the volumes. The operator takes care of enabling CBT for the virtual machines. The plugin will be responsible to enable CBT
for the volumes.

Currently, only toggling CBT configuration in guest clusters (vSphere specific) is supported. CBT will be enabled/disabled
for all the guest clusters in given supervisor cluster. Going forward, more fine grained control will be added to support
setting CBT on a per guest cluster level.

### CBT support using Mutating Webhook

At a high level, CBT support for guest clusters is implemented using a mutating webhook.

- When the CBT setting is configured, the setting is updated in all the existing guest cluster virtual machines. 
- A webhook in the velero operator watches for the creation of new guest clusters. When a new guest cluster virtual machine
is created, the CBT flag is set based on the current CBT setting for guest clusters.
- If velero is uninstalled in the supervisor cluster, the CBT setting is disabled in all the existing guest cluster virtual machines.

**NOTE**: As a best practice, uninstall velero in the supervisor cluster before disabling the velero vSphere operator.
If the velero vSphere operator is disabled before uninstalling velero, clearing the CBT setting is best effort, and not guaranteed. 
