#!/bin/bash
#
# Copyright 2017 the Velero contributors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o errexit
set -o nounset
set -o pipefail
set -o xtrace

if [[ -z "${GOPATH}" ]]; then
  GOPATH=~/go
fi

if [[ ! -d "${GOPATH}/src/k8s.io/code-generator" ]]; then
  echo "k8s.io/code-generator missing from GOPATH"
  exit 1
fi

if ! command -v controller-gen > /dev/null; then
  echo "controller-gen is missing"
  echo "please retry after running the following command locally: go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.4.0"
  exit 1
fi

${GOPATH}/src/k8s.io/code-generator/generate-groups.sh \
  all \
  github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/generated \
  github.com/vmware-tanzu/velero-plugin-for-vsphere/pkg/apis \
  "datamover:v1alpha1 backupdriver:v1alpha1" \
  --go-header-file ${GOPATH}/src/github.com/vmware-tanzu/velero-plugin-for-vsphere/hack/boilerplate.go.txt \
  $@

controller-gen \
  crd \
  crd:crdVersions=v1 \
  output:dir=pkg/generated/crds/manifests \
  paths=./pkg/apis/...

go generate ./pkg/generated/crds
