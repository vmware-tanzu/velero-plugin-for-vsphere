package vsphere

import (
	"context"
	vslmtypes "github.com/vmware/govmomi/vslm/types"
	"testing"
)

/*
	Steps

	0. Set KUBECONFIG to the kubeconfig of vanilla or supervisor cluster.
	1. Retrieve the vcenter connection parameters from the secret and create VcenterConfig.
	2. Retrieve the vcenter center instance for the config.
	3. Invoke GetVslmManager to associate the new vcenter instance.

	Expected Output:
	1. No errors during the workflow.

*/
func TestVslmManager_GetVslmManager(t *testing.T) {
	log := GetLogger()
	ctx := context.Background()
	virtualCenterConfig := RetrieveVirtualCenterConfig(t, log)
	log.Infof("Successfully retrieved VirtualCenterConfig : %+v", virtualCenterConfig)

	virtualCenter, vslmClient, _, err := GetVirtualCenter(ctx, virtualCenterConfig, log)
	if err != nil {
		t.Fatalf("Failed to register virtual center err: %+v", err)
	}
	defer func() {
		err = virtualCenter.Disconnect(ctx)
		if err != nil {
			log.Warnf("Was unable to disconnect vc clients as part of cleanup.")
		}
	}()
	_, err = GetVslmManager(ctx, virtualCenter, vslmClient, log)
	if err != nil {
		t.Fatalf("Failed to retrieve vslm manager.")
	}
	log.Infof("PASS: Successfully retrieved vslm manager.")
}

/*
	Steps

	0. Set KUBECONFIG to the kubeconfig of vanilla or supervisor cluster.
	1. Retrieve the vcenter connection parameters from the secret and create VcenterConfig.
	2. Retrieve the vcenter center instance for the config.
	3. Invoke GetVslmManager to associate the new vcenter instance.
	4. Invoke VslmListObjectsForSpec to verify that APIs can be invoked.

	Expected Output:
	1. No errors during the workflow.

*/
func TestVslmManager_VslmApiInvocation(t *testing.T) {
	log := GetLogger()
	ctx := context.Background()
	virtualCenterConfig := RetrieveVirtualCenterConfig(t, log)
	log.Infof("Successfully retrieved VirtualCenterConfig : %+v", virtualCenterConfig)

	virtualCenter, vslmClient, _, err := GetVirtualCenter(ctx, virtualCenterConfig, log)
	if err != nil {
		t.Fatalf("Failed to register virtual center err: %+v", err)
	}
	defer func() {
		err = virtualCenter.Disconnect(ctx)
		if err != nil {
			log.Warnf("Was unable to disconnect vc clients as part of cleanup.")
		}
	}()
	vslmManager, err := GetVslmManager(ctx, virtualCenter, vslmClient, log)
	if err != nil {
		t.Fatalf("Failed to retrieve vslm manager.")
	}
	// Invoke vslm APIs.
	spec := vslmtypes.VslmVsoVStorageObjectQuerySpec{
		QueryField:    "createTime",
		QueryOperator: "greaterThan",
		QueryValue:    []string{"0"},
	}
	res, err := vslmManager.ListObjectsForSpec(ctx, []vslmtypes.VslmVsoVStorageObjectQuerySpec{spec}, 1000)
	if err != nil {
		t.Fatalf("Failed to invoke VSLM APIs.")
	}
	log.Infof("PASS: Successfully invoked VSLM List API, retrieved %d FCDs", len(res.QueryResults))
}

/*
	Steps

	0. Set KUBECONFIG to the kubeconfig of vanilla or supervisor cluster.
	1. Retrieve the vcenter connection parameters from the secret and create VcenterConfig.
	2. Retrieve the vcenter center instance for the config.
	3. Invoke Disconnect on the vcenter instance.
	4. Use the vslm manager to invoke any of the vslm apis.

	Expected Output:
	1. No Error.

*/
func TestVslmManager_VslmApiInvocationAfterDisconnect(t *testing.T) {
	log := GetLogger()
	ctx := context.Background()
	virtualCenterConfig := RetrieveVirtualCenterConfig(t, log)
	log.Infof("Successfully retrieved VirtualCenterConfig : %+v", virtualCenterConfig)

	virtualCenter, vslmClient, _, err := GetVirtualCenter(ctx, virtualCenterConfig, log)
	if err != nil {
		t.Fatalf("Failed to register virtual center err: %+v", err)
	}
	vslmManager, err := GetVslmManager(ctx, virtualCenter, vslmClient, log)
	if vslmManager == nil {
		t.Fatalf("Failed to retrieve vslm manager.")
	}
	err = virtualCenter.Disconnect(ctx)
	if err != nil {
		t.Fatalf("Failed to disconnect virtual center.")
	}
	// Invoke vslm APIs.
	spec := vslmtypes.VslmVsoVStorageObjectQuerySpec{
		QueryField:    "createTime",
		QueryOperator: "greaterThan",
		QueryValue:    []string{"0"},
	}
	res, err := vslmManager.ListObjectsForSpec(ctx, []vslmtypes.VslmVsoVStorageObjectQuerySpec{spec}, 1000)
	if err != nil {
		t.Fatalf("Failed to invoke VSLM APIs.")
	}
	log.Infof("PASS: Successfully invoked VSLM List API even after disconnect, retrieved %d FCDs", len(res.QueryResults))
}

/*
	Steps

	0. Set KUBECONFIG to the kubeconfig of vanilla or supervisor cluster.
	1. Retrieve the vcenter connection parameters from the secret and create VcenterConfig.
	2. Retrieve the vcenter center instance for the config.
	3. Invoke GetVslmManager to associate the new vcenter instance.
	4. Invoke any of the vslm APIs to trigger vcenter and vslm connect.
	5. Create a VcenterConfig with alternate credentials.
	6. Invoke Disconnect.
	7. Retrieve the vcenter center instance with a new VcenterConfig.
	8. Use the returned vcenter instance to Invoke ResetManager on the vslm manager.
	9. Invoke any of the vslm apis to ensure no errors.

	Expected Output:
	1. No Error.

*/
func TestVslmManager_ResetManager(t *testing.T) {
	log := GetLogger()
	ctx := context.Background()
	virtualCenterConfig := RetrieveVirtualCenterConfig(t, log)
	log.Infof("Successfully retrieved VirtualCenterConfig : %+v", virtualCenterConfig)

	virtualCenter, vslmClient, _, err := GetVirtualCenter(ctx, virtualCenterConfig, log)
	if err != nil {
		t.Fatalf("Failed to register virtual center err: %+v", err)
	}
	vslmManager, err := GetVslmManager(ctx, virtualCenter, vslmClient, log)
	if err != nil {
		t.Fatalf("Failed to retrieve vslm manager.")
	}
	err = virtualCenter.Disconnect(ctx)
	if err != nil {
		t.Fatalf("Was unable to disconnect vc clients, err: %+v", err)
	}
	// Using admin as alternate vcenter config
	adminUser := "administrator@vsphere.local"
	adminPass := "Admin!23"
	virtualCenterConfig.Username = adminUser
	virtualCenterConfig.Password = adminPass
	newVirtualCenter, vslmClient, _, err := GetVirtualCenter(ctx, virtualCenterConfig, log)
	if err != nil {
		t.Fatalf("Failed to register virtual center err: %+v", err)
	}
	vslmManager.ResetManager(newVirtualCenter, vslmClient)
	// Invoke vslm APIs.
	spec := vslmtypes.VslmVsoVStorageObjectQuerySpec{
		QueryField:    "createTime",
		QueryOperator: "greaterThan",
		QueryValue:    []string{"0"},
	}
	res, err := vslmManager.ListObjectsForSpec(ctx, []vslmtypes.VslmVsoVStorageObjectQuerySpec{spec}, 1000)
	if err != nil {
		t.Fatalf("Failed to invoke VSLM APIs.")
	}
	log.Infof("PASS: Successfully invoked VSLM List API after registering new vcenter, retrieved %d FCDs", len(res.QueryResults))
}
