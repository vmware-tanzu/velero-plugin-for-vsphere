# vSphere with Kubernetes notes

## Restricted resources
Certain resources with the vSphere with Kubernetes Supervisor Cluster need to be created by the Supervisor Cluster and
cannot be restored. The Velero Plugin for vSphere blocks backup and restore of these resources and will generate errors
if an attempt is made to backup or restore these resources and the backup or restore will be marked as "Partially Failed".
To avoid these errors, exclude the resources from your backups. The current list of blocked resources is:

### vSphere with Kubernetes Supervisor Cluster resources
	agentinstalls.installers.tmc.cloud.vmware.com               
	certificaterequests.cert-manager.io                         
	certificates.cert-manager.io                                
	challenges.acme.cert-manager.io                             
	clusterissuers.cert-manager.io                              
	clusterresourcesetbindings.addons.cluster.x-k8s.io          
	clusterresourcesets.addons.cluster.x-k8s.io                 
	clusters.cluster.x-k8s.io                                   
	cnsnodevmattachments.cns.vmware.com                         
	cnsregistervolumes.cns.vmware.com                           
	cnsvolumemetadatas.cns.vmware.com                           
	compatibilities.run.tanzu.vmware.com                        
	contentlibraryproviders.vmoperator.vmware.com               
	contentsources.vmoperator.vmware.com                        
	imagedisks.imagecontroller.vmware.com                       
	installoptions.appplatform.wcp.vmware.com                   
	installrequirements.appplatform.wcp.vmware.com              
	issuers.cert-manager.io                                     
	kubeadmconfigs.bootstrap.cluster.x-k8s.io                   
	kubeadmconfigtemplates.bootstrap.cluster.x-k8s.io           
	kubeadmcontrolplanes.controlplane.cluster.x-k8s.io          
	kuberneteslicenses.licenseoperator.vmware.com               
	loadbalancers.vmware.com                                    
	machinedeployments.cluster.x-k8s.io                         
	machinehealthchecks.cluster.x-k8s.io                        
	machinepools.exp.cluster.x-k8s.io                           
	machines.cluster.x-k8s.io                                   
	machinesets.cluster.x-k8s.io                                
	members.registryagent.vmware.com                            
	ncpconfigs.nsx.vmware.com                                   
	network-attachment-definitions.k8s.cni.cncf.io              
	nsxerrors.nsx.vmware.com                                    
	nsxlbmonitors.vmware.com                                    
	nsxlocks.nsx.vmware.com                                     
	nsxnetworkinterfaces.nsx.vmware.com                         
	orders.acme.cert-manager.io                                 
	projects.registryagent.vmware.com                           
	providerserviceaccounts.run.tanzu.vmware.com                
	registries.registryagent.vmware.com                         
	storagepolicies.appplatform.wcp.vmware.com                  
	storagepools.cns.vmware.com                                 
	supervisorservices.appplatform.wcp.vmware.com               
	tanzukubernetesclusters.run.tanzu.vmware.com                
	tanzukubernetesreleases.run.tanzu.vmware.com                
	tkgserviceconfigurations.run.tanzu.vmware.com               
	vcuiplugins.appplatform.wcp.vmware.com                      
	veleroservices.veleroappoperator.vmware.com                 
	virtualmachineclasses.vmoperator.vmware.com                 
	virtualmachineimages.vmoperator.vmware.com                  
	virtualmachineservices.vmoperator.vmware.com                
	virtualmachinesetresourcepolicies.vmoperator.vmware.com     
	virtualmachines.vmoperator.vmware.com                       
	virtualnetworkinterfaces.vmware.com                         
	virtualnetworks.vmware.com                                  
	wcpclusters.infrastructure.cluster.vmware.com               
	wcpmachines.infrastructure.cluster.vmware.com               
	wcpmachinetemplates.infrastructure.cluster.vmware.com       

For example, the backup of a supervisor namespace with Tanzu Kubernetes Grid Service will fail since it contains
some of restricted resources listed above.

## Supervisor Cluster resources handled during restore

The following resources are handled during the restore on the
Supervisor Cluster.
  * `images.imagecontroller.vmware.com`
    The image resource is backed up everytime when a container
    is backed up on the Supervisor Cluster. It will be skipped
    at restore time.
  * `pods`
    We need to remove some metadata from the Pod resource on the
    Supervisor Cluster, i.e., annotation "vmware-system-vm-uuid"
    before the restore as the existing VM UUID is associated with
    the old VM that does not exist any more. The following annotations
    will be removed from the Pod resource on the Supervisor Cluster.
    * kubernetes.io/psp
    * mac
    * vlan
    * vmware-system-ephemeral-disk-uuid
    * vmware-system-image-references
    * vmware-system-vm-moid
    * vmware-system-vm-uuid



## vSphere Plugin resources

The Velero Plug-in for vSphere also has internal resources. These are 
automatically tagged with the Velero velero.io/exclude-from-backup label
and will not be part of backups. Additionally, the plugin will block these
as well, if you see errors related to these resource, the exclude-from-backup
label has probably been removed from them.

    backuprepositories.backupdriver.cnsdp.vmware.com     
    backuprepositoryclaims.backupdriver.cnsdp.vmware.com 
    clonefromsnapshots.backupdriver.cnsdp.vmware.com     
    deletesnapshots.backupdriver.cnsdp.vmware.com        
    downloads.datamover.cnsdp.vmware.com              
    snapshots.backupdriver.cnsdp.vmware.com              
    uploads.datamover.cnsdp.vmware.com         