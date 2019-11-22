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

// Code generated by lister-gen. DO NOT EDIT.

package v1

import (
	v1 "github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis/veleroplugin/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// DownloadLister helps list Downloads.
type DownloadLister interface {
	// List lists all Downloads in the indexer.
	List(selector labels.Selector) (ret []*v1.Download, err error)
	// Downloads returns an object that can list and get Downloads.
	Downloads(namespace string) DownloadNamespaceLister
	DownloadListerExpansion
}

// downloadLister implements the DownloadLister interface.
type downloadLister struct {
	indexer cache.Indexer
}

// NewDownloadLister returns a new DownloadLister.
func NewDownloadLister(indexer cache.Indexer) DownloadLister {
	return &downloadLister{indexer: indexer}
}

// List lists all Downloads in the indexer.
func (s *downloadLister) List(selector labels.Selector) (ret []*v1.Download, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1.Download))
	})
	return ret, err
}

// Downloads returns an object that can list and get Downloads.
func (s *downloadLister) Downloads(namespace string) DownloadNamespaceLister {
	return downloadNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// DownloadNamespaceLister helps list and get Downloads.
type DownloadNamespaceLister interface {
	// List lists all Downloads in the indexer for a given namespace.
	List(selector labels.Selector) (ret []*v1.Download, err error)
	// Get retrieves the Download from the indexer for a given namespace and name.
	Get(name string) (*v1.Download, error)
	DownloadNamespaceListerExpansion
}

// downloadNamespaceLister implements the DownloadNamespaceLister
// interface.
type downloadNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all Downloads in the indexer for a given namespace.
func (s downloadNamespaceLister) List(selector labels.Selector) (ret []*v1.Download, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1.Download))
	})
	return ret, err
}

// Get retrieves the Download from the indexer for a given namespace and name.
func (s downloadNamespaceLister) Get(name string) (*v1.Download, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1.Resource("download"), name)
	}
	return obj.(*v1.Download), nil
}
