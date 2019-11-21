/*
Copyright 2017 the Velero contributors.

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

package downloadrequest

import (
	"compress/gzip"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"

	v1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	velerov1client "github.com/vmware-tanzu/velero/pkg/generated/clientset/versioned/typed/velero/v1"
)

// ErrNotFound is exported for external packages to check for when a file is
// not found
var ErrNotFound = errors.New("file not found")

func Stream(client velerov1client.DownloadRequestsGetter, namespace, name string, kind v1.DownloadTargetKind, w io.Writer, timeout time.Duration, insecureSkipTLSVerify bool) error {
	req := &v1.DownloadRequest{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      fmt.Sprintf("%s-%s", name, time.Now().Format("20060102150405")),
		},
		Spec: v1.DownloadRequestSpec{
			Target: v1.DownloadTarget{
				Kind: kind,
				Name: name,
			},
		},
	}

	req, err := client.DownloadRequests(namespace).Create(req)
	if err != nil {
		return errors.WithStack(err)
	}
	defer client.DownloadRequests(namespace).Delete(req.Name, nil)

	listOptions := metav1.ListOptions{
		// TODO: once the minimum supported Kubernetes version is v1.9.0, uncomment the following line.
		// See http://issue.k8s.io/51046 for details.
		//FieldSelector:   "metadata.name=" + req.Name
		ResourceVersion: req.ResourceVersion,
	}
	watcher, err := client.DownloadRequests(namespace).Watch(listOptions)
	if err != nil {
		return errors.WithStack(err)
	}
	defer watcher.Stop()

	expired := time.NewTimer(timeout)
	defer expired.Stop()

Loop:
	for {
		select {
		case <-expired.C:
			return errors.New("timed out waiting for download URL")
		case e := <-watcher.ResultChan():
			updated, ok := e.Object.(*v1.DownloadRequest)
			if !ok {
				return errors.Errorf("unexpected type %T", e.Object)
			}

			// TODO: once the minimum supported Kubernetes version is v1.9.0, remove the following check.
			// See http://issue.k8s.io/51046 for details.
			if updated.Name != req.Name {
				continue
			}

			switch e.Type {
			case watch.Deleted:
				errors.New("download request was unexpectedly deleted")
			case watch.Modified:
				if updated.Status.DownloadURL != "" {
					req = updated
					break Loop
				}
			}
		}
	}

	if req.Status.DownloadURL == "" {
		return ErrNotFound
	}

	httpClient := new(http.Client)
	if insecureSkipTLSVerify {
		httpClient.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	httpReq, err := http.NewRequest("GET", req.Status.DownloadURL, nil)
	if err != nil {
		return err
	}

	// Manually set this header so the net/http library does not automatically try to decompress. We
	// need to handle this manually because it's not currently possible to set the MIME type for the
	// pre-signed URLs for GCP or Azure.
	httpReq.Header.Set("Accept-Encoding", "gzip")

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		if urlErr, ok := err.(*url.Error); ok {
			if _, ok := urlErr.Err.(x509.UnknownAuthorityError); ok {
				return fmt.Errorf(err.Error() + "\n\nThe --insecure-skip-tls-verify flag can also be used to accept any TLS certificate for the download, but it is susceptible to man-in-the-middle attacks.")
			}
		}
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return errors.Wrapf(err, "request failed: unable to decode response body")
		}

		if resp.StatusCode == http.StatusNotFound {
			return ErrNotFound
		}

		return errors.Errorf("request failed: %v", string(body))
	}

	reader := resp.Body
	if kind != v1.DownloadTargetKindBackupContents {
		// need to decompress logs
		gzipReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return err
		}
		defer gzipReader.Close()
		reader = gzipReader
	}

	_, err = io.Copy(w, reader)
	return err
}
