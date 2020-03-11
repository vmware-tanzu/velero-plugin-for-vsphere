# Velero Plugins for vSphere 

## First Tips
Please place the git repo under $GOPATH/src/github.com/vmware-tanzu/velero-plugin-for-vsphere.

## Kinds of Plugins

Velero currently supports the following kinds of plugins:

- **Block Store** - creates snapshots from volumes (during a backup) and volumes from snapshots (during a restore).

## Building the plugin

To build the plugin, run

```bash
$ make
```

To build the image, run

```bash
$ make container
```

This builds an image tagged as `velero/velero-plugin-for-vsphere`. If you want to specify a
different name, run

```bash
$ make container IMAGE=your-repo/your-name:here
```

## Deploying the plugins


To deploy your plugin image to an Velero server:

1. In the CSI setup, it is required to use the hostNetwork option for the deployment. Run `kubectl patch deploy/velero -n velero --patch "$(cat deployment/patch-deployment-hostNetwork.yaml)"`.
2. This plugin is dependent on shared library during plugin install. Starting from velero v1.2.0, the env `LD_LIBRARY_PATH` is set by default. If not already exist, manually edit `deployment/velero` to add `LD_LIBRARY_PATH` environment variable or use `deployment/create-deployment-for-plugin.yaml` to update the deployment.
```yaml
spec:
  template:
    spec:
      containers:
      - args:
        name: velero
        env:
        # Add LD_LIBRARY_PATH to environment variable
        - name: LD_LIBRARY_PATH
          value: /plugins
```
3. Make sure your image is pushed to a registry that is accessible to your cluster's nodes.
4. Run `velero plugin add <image>`, e.g. `velero plugin add velero/vsphere-plugin-for-velero`

## Using the plugins

When the plugin is deployed, it is only made available to use. To make the plugin effective, you must modify your configuration:

Volume snapshot storage:

1. Run `kubectl edit volumesnapshotlocation <location-name> -n <velero-namespace>` e.g. `kubectl edit volumesnapshotlocation default -n velero` OR `velero snapshot-location create <location-name> --provider <provider-name>`
2. Change the value of `spec.provider` to enable a **Block Store** plugin
3. Save and quit. The plugin will be used for the next `backup/restore`

## HOWTO

To run with the plugin, do the following:

1. Run `velero snapshot-location create example-default --provider velero.io/vsphere`
2. Run `kubectl edit deployment/velero -n <velero-namespace>`
3. Change the value of `spec.template.spec.args` to look like the following:

    ```yaml
          - args:
            - server
            - --default-volume-snapshot-locations
            - velero.io/vsphere:example-default
    ```

    Note: users can also run `kubectl apply -f deployment/create-deployment-for-plugin.yaml` to configure the deployment for the plugin, which is equivalent to steps 1 - 3 mentioned above. 
4. Run `kubectl apply -f examples/demo-app.yaml` to apply a sample nginx application that uses the example block store plugin. ***Note***: This example works best on a virtual machine, as it uses the host's `/tmp` directory for data storage.
