#!/bin/bash

# Copyright 2020 The Velero contributors.
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

if [ ! -d "${GOPATH}/src/${VDDK_LIBS}" ]
then
    echo "Error: ${GOPATH}/src/${VDDK_LIBS} cannot find vddk libs in path, please reference to: https://github.com/vmware-tanzu/astrolabe/tree/master/vendor/github.com/vmware/gvddk#dependency"
fi

export LD_LIBRARY_PATH=${GOPATH}/src/${VDDK_LIBS}

TARGETS=(
  ./pkg/...
)

if [[ ${#@} -ne 0 ]]; then
  TARGETS=("$@")
fi

echo "Running tests:" "${TARGETS[@]}"

if [[ -n "${GOFLAGS:-}" ]]; then
  echo "GOFLAGS: ${GOFLAGS}"
fi

go test -installsuffix "static" -timeout 60s "${TARGETS[@]}"
echo "Success!"
