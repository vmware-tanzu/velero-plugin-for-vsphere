/*
Copyright the Velero contributors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	backupdriverv1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/backupdriver/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeBackupRepositoryClaims implements BackupRepositoryClaimInterface
type FakeBackupRepositoryClaims struct {
	Fake *FakeBackupdriverV1
	ns   string
}

var backuprepositoryclaimsResource = schema.GroupVersionResource{Group: "backupdriver.io", Version: "v1", Resource: "backuprepositoryclaims"}

var backuprepositoryclaimsKind = schema.GroupVersionKind{Group: "backupdriver.io", Version: "v1", Kind: "BackupRepositoryClaim"}

// Get takes name of the backupRepositoryClaim, and returns the corresponding backupRepositoryClaim object, and an error if there is any.
func (c *FakeBackupRepositoryClaims) Get(name string, options v1.GetOptions) (result *backupdriverv1.BackupRepositoryClaim, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewGetAction(backuprepositoryclaimsResource, c.ns, name), &backupdriverv1.BackupRepositoryClaim{})

	if obj == nil {
		return nil, err
	}
	return obj.(*backupdriverv1.BackupRepositoryClaim), err
}

// List takes label and field selectors, and returns the list of BackupRepositoryClaims that match those selectors.
func (c *FakeBackupRepositoryClaims) List(opts v1.ListOptions) (result *backupdriverv1.BackupRepositoryClaimList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewListAction(backuprepositoryclaimsResource, backuprepositoryclaimsKind, c.ns, opts), &backupdriverv1.BackupRepositoryClaimList{})

	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &backupdriverv1.BackupRepositoryClaimList{ListMeta: obj.(*backupdriverv1.BackupRepositoryClaimList).ListMeta}
	for _, item := range obj.(*backupdriverv1.BackupRepositoryClaimList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested backupRepositoryClaims.
func (c *FakeBackupRepositoryClaims) Watch(opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewWatchAction(backuprepositoryclaimsResource, c.ns, opts))

}

// Create takes the representation of a backupRepositoryClaim and creates it.  Returns the server's representation of the backupRepositoryClaim, and an error, if there is any.
func (c *FakeBackupRepositoryClaims) Create(backupRepositoryClaim *backupdriverv1.BackupRepositoryClaim) (result *backupdriverv1.BackupRepositoryClaim, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewCreateAction(backuprepositoryclaimsResource, c.ns, backupRepositoryClaim), &backupdriverv1.BackupRepositoryClaim{})

	if obj == nil {
		return nil, err
	}
	return obj.(*backupdriverv1.BackupRepositoryClaim), err
}

// Update takes the representation of a backupRepositoryClaim and updates it. Returns the server's representation of the backupRepositoryClaim, and an error, if there is any.
func (c *FakeBackupRepositoryClaims) Update(backupRepositoryClaim *backupdriverv1.BackupRepositoryClaim) (result *backupdriverv1.BackupRepositoryClaim, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewUpdateAction(backuprepositoryclaimsResource, c.ns, backupRepositoryClaim), &backupdriverv1.BackupRepositoryClaim{})

	if obj == nil {
		return nil, err
	}
	return obj.(*backupdriverv1.BackupRepositoryClaim), err
}

// Delete takes name of the backupRepositoryClaim and deletes it. Returns an error if one occurs.
func (c *FakeBackupRepositoryClaims) Delete(name string, options *v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewDeleteAction(backuprepositoryclaimsResource, c.ns, name), &backupdriverv1.BackupRepositoryClaim{})

	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeBackupRepositoryClaims) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	action := testing.NewDeleteCollectionAction(backuprepositoryclaimsResource, c.ns, listOptions)

	_, err := c.Fake.Invokes(action, &backupdriverv1.BackupRepositoryClaimList{})
	return err
}

// Patch applies the patch and returns the patched backupRepositoryClaim.
func (c *FakeBackupRepositoryClaims) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *backupdriverv1.BackupRepositoryClaim, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewPatchSubresourceAction(backuprepositoryclaimsResource, c.ns, name, pt, data, subresources...), &backupdriverv1.BackupRepositoryClaim{})

	if obj == nil {
		return nil, err
	}
	return obj.(*backupdriverv1.BackupRepositoryClaim), err
}