# StorageClass Mapping

Velero plugin for vsphere v1.1.0 and above supports changing the storage class persistent volumes during restores, 
by configuring a storage class mapping in config map in the Velero namespace. For more details, please refer to [Velero Restore Reference](https://velero.io/docs/v1.5/restore-reference/).

Here is an example for ConfigMap in YAML, which contains <old-storage-class>:<new-storage-class> mapping pairs.

```bash
apiVersion: v1
kind: ConfigMap
metadata:
  # any name can be used; Velero uses the labels (below)
  # to identify it rather than the name
  name: change-storage-class-config
  # must be in the velero namespace
  namespace: velero
  # the below labels should be used verbatim in your
  # ConfigMap.
  labels:
    # this value-less label identifies the ConfigMap as
    # config for a plugin (i.e. the built-in restore item action plugin)
    velero.io/plugin-config: ""
    # this label identifies the name and kind of plugin
    # that this ConfigMap is for.
    velero.io/change-storage-class: RestoreItemAction
data:
  # add 1+ key-value pairs here, where the key is the old
  # storage class name and the value is the new storage
  # class name.
  <old-storage-class>: <new-storage-class>
```

Storage class is required for restore. If no storage class is specified in the PVC during backup, user can specify 
"com.vmware.cnsdp.emptystorageclass" as the old storage class name to map to a new existing storage class name at restore time.

```bash
apiVersion: v1
kind: ConfigMap
metadata:
...
data:
  com.vmware.cnsdp.emptystorageclass: <new-storage-class>
```