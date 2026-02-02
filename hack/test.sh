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

if [ -z "${TARGETS}" ]; then
    echo "TARGETS must be set"
    exit 1
fi

if [ ! -z "${TIMEOUT}" ]; then
    TIMEOUT="-timeout=${TIMEOUT}"
fi

if [ ! -z "${VERBOSE}" ]; then
    VERBOSE="-v"
fi

if [ ! -z "${DISABLE_CACHE}" ]; then
    DISABLE_CACHE="-count=1"
fi

if [ ! -z "${RUN_SINGLE_CASE}" ]; then
    RUN_SINGLE_CASE="-run=${RUN_SINGLE_CASE}"
fi

echo "Running tests:" "${TARGETS}"

if [[ -n "${GOFLAGS:-}" ]]; then
  echo "GOFLAGS: ${GOFLAGS}"
fi

echo go test "${TARGETS}" ${TIMEOUT} ${RUN_SINGLE_CASE} ${VERBOSE} ${DISABLE_CACHE}
go test -gcflags="-l" "${TARGETS}" ${TIMEOUT} ${RUN_SINGLE_CASE} ${VERBOSE} ${DISABLE_CACHE}
echo "Success!"
