#!/bin/bash

# Copyright 2016 The Kubernetes Authors.
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

if [ -z "${PKG}" ]; then
    echo "PKG must be set"
    exit 1
fi
if [ -z "${BIN}" ]; then
    echo "BIN must be set"
    exit 1
fi
if [ -z "${GOOS}" ]; then
    echo "GOOS must be set"
    exit 1
fi
if [ -z "${GOARCH}" ]; then
    echo "GOARCH must be set"
    exit 1
fi
if [ -z "${VERSION}" ]; then
    echo "VERSION must be set"
    exit 1
fi
if [ -z "${REGISTRY}" ]; then
    echo "REGISTRY must be set"
    exit 1
fi
if [ -z "${GIT_SHA}" ]; then
    echo "GIT_SHA must be set"
    exit 1
fi


export GOEXPERIMENT=boringcrypto

if [[ -z "${GIT_DIRTY}" ]]; then
  GIT_TREE_STATE=clean
else
  GIT_TREE_STATE=dirty
fi

LDFLAGS="-X ${PKG}/pkg/buildinfo.Version=${VERSION}"
LDFLAGS="${LDFLAGS} -X ${PKG}/pkg/buildinfo.Registry=${REGISTRY}"
LDFLAGS="${LDFLAGS} -X ${PKG}/pkg/buildinfo.GitSHA=${GIT_SHA}"
LDFLAGS="${LDFLAGS} -X ${PKG}/pkg/buildinfo.GitTreeState=${GIT_TREE_STATE}"

if [[ -z "${OUTPUT_DIR:-}" ]]; then
  OUTPUT_DIR=.
fi
OUTPUT=${OUTPUT_DIR}/${BIN}
if [[ "${GOOS}" = "windows" ]]; then
  OUTPUT="${OUTPUT}.exe"
fi

GO111MODULES=on go build \
    -o ${OUTPUT} \
    -installsuffix "static" \
    -ldflags "${LDFLAGS}" \
    ${PKG}/cmd/${BIN}
