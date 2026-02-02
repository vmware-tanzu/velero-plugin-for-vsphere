#
# Copyright 2019 VMware, Inc..
# SPDX-License-Identifier: Apache-2.0
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
#

# This repo's root import path (under GOPATH).
PKG := github.com/vmware-tanzu/velero-plugin-for-vsphere


# The binary to build (just the basename).
PLUGIN_BIN ?= velero-plugin-for-vsphere
BIN ?= $(PLUGIN_BIN)

RELEASE_REGISTRY = ?vsphereveleroplugin
REGISTRY ?= dpcpinternal
PLUGIN_IMAGE ?= $(REGISTRY)/$(PLUGIN_BIN)

# The default container runtime is docker, but others are supported (e.g. podman, nerdctl).
DOCKER ?= docker

# Which architecture to build - see $(ALL_ARCH) for options.
# if the 'local' rule is being run, detect the ARCH from 'go env'
# if it wasn't specified by the caller.
local : ARCH ?= $(shell go env GOOS)-$(shell go env GOARCH)
ARCH ?= linux-amd64

# VERSION is <git branch>-<git commit>-<date
# Uses ifndef instead of ?= so that date will only be evaluated once, not each time VERSION is used
ifndef VERSION
VERSION := $(shell echo `git rev-parse --abbrev-ref HEAD`-`git log -1 --pretty=format:%h`-`date "+%d.%b.%Y.%H.%M.%S"`)
endif

# set git sha and tree state
GIT_SHA = $(shell git rev-parse HEAD)
GIT_DIRTY = $(shell git status --porcelain 2> /dev/null)

platform_temp = $(subst -, ,$(ARCH))
GOOS = $(word 1, $(platform_temp))
GOARCH = $(word 2, $(platform_temp))
SOURCE_PHOTON_IMAGE ?= golang:1.25
BUILDER_IMAGE := $(SOURCE_PHOTON_IMAGE)
PLUGIN_DOCKERFILE ?= Dockerfile-plugin

all: plugin

plugin:
	@echo "making: $@"
	$(MAKE) build BIN=$(PLUGIN_BIN) VERSION=$(VERSION)

local: build-dirs
	GOOS=$(GOOS) \
	GOARCH=$(GOARCH) \
	PKG=$(PKG) \
	BIN=$(BIN) \
	REGISTRY=$(REGISTRY) \
	VERSION=$(VERSION) \
	GIT_SHA=$(GIT_SHA) \
	GIT_DIRTY="$(GIT_DIRTY)" \
	OUTPUT_DIR=$$(pwd)/_output/bin/$(GOOS)/$(GOARCH) \
	GO111MODULE=on \
	GOFLAGS=-mod=readonly \
	./hack/build.sh

build: _output/bin/$(GOOS)/$(GOARCH)/$(BIN)

_output/bin/$(GOOS)/$(GOARCH)/$(BIN): build-dirs
	@echo "building: $@"
	$(MAKE) shell CMD="-c '\
		GOOS=$(GOOS) \
		GOARCH=$(GOARCH) \
		REGISTRY=$(REGISTRY) \
		VERSION=$(VERSION) \
		PKG=$(PKG) \
		BIN=$(BIN) \
		GIT_SHA=$(GIT_SHA) \
		GIT_DIRTY=\"$(GIT_DIRTY)\" \
		OUTPUT_DIR=/output/$(GOOS)/$(GOARCH) \
		GO111MODULE=on \
		GOFLAGS=-mod=readonly \
		./hack/build.sh'"

TTY := $(shell tty -s && echo "-t")

shell: build-dirs
	@echo "running $(DOCKER): $@"
	$(DOCKER) run \
		--platform $(GOOS)/$(GOARCH) \
		-e GOFLAGS \
		-i $(TTY) \
		--rm \
		-u $$(id -u):$$(id -g) \
		-v $$(pwd)/.go/pkg:/go/pkg:delegated \
		-v $$(pwd)/.go/src:/go/src:delegated \
		-v $$(pwd)/.go/std:/go/std:delegated \
		-v $$(pwd):/go/src/$(PKG):delegated \
		-v "$$(pwd)/_output/bin:/output:delegated" \
		-v $$(pwd)/.go/std/$(GOOS)/$(GOARCH):/usr/local/go/pkg/$(GOOS)/$(GOARCH)_static:delegated \
		-v "$$(pwd)/.go/go-build:/.cache/go-build:delegated" \
		-e CGO_ENABLED=1 -e GOEXPERIMENT=boringcrypto \
		-e GOPATH=/go \
		-w /go/src/$(PKG) \
		$(BUILDER_IMAGE) \
		/bin/sh $(CMD)

build-dirs:
	@mkdir -p _output/bin/$(GOOS)/$(GOARCH)
	@mkdir -p .go/src/$(PKG) .go/pkg .go/bin .go/std/$(GOOS)/$(GOARCH) .go/go-build


container-name:
	@echo "container: $(IMAGE):$(VERSION)"

copy-install-script:
	cp $$(pwd)/scripts/install.sh _output/bin/$(GOOS)/$(GOARCH)

build-container: container-name
	cp $(DOCKERFILE) _output/bin/$(GOOS)/$(GOARCH)/$(DOCKERFILE)
	$(DOCKER) build --platform $(GOOS)/$(GOARCH) -t $(IMAGE):$(VERSION) -f _output/bin/$(GOOS)/$(GOARCH)/$(DOCKERFILE) _output

plugin-container: all copy-install-script
	$(MAKE) build-container IMAGE=$(PLUGIN_IMAGE) DOCKERFILE=$(PLUGIN_DOCKERFILE) VERSION=$(VERSION)

container: plugin-container

push-plugin: plugin-container
	$(DOCKER) push $(PLUGIN_IMAGE):$(VERSION)

push: push-plugin

QUALIFIED_TAG ?=
RELEASE_TAG ?= latest
release:
ifneq (,$(QUALIFIED_TAG))
	$(DOCKER) pull $(PLUGIN_IMAGE):$(QUALIFIED_TAG)
	$(DOCKER) tag $(PLUGIN_IMAGE):$(QUALIFIED_TAG) $(RELEASE_REGISTRY)/$(PLUGIN_BIN):$(RELEASE_TAG)
	$(DOCKER) push $(RELEASE_REGISTRY)/$(PLUGIN_BIN):$(RELEASE_TAG)
endif

TARGETS ?= ./pkg/...
TIMEOUT ?= 300s
VERBOSE ?= # empty by default
DISABLE_CACHE ?= # empty by default
RUN_SINGLE_CASE ?= # empty by default
test: build-dirs
	@$(MAKE) shell CMD="-c '\
	     TARGETS=$(TARGETS) \
	     TIMEOUT=$(TIMEOUT) \
	     VERBOSE=$(VERBOSE) \
	     DISABLE_CACHE=$(DISABLE_CACHE) \
	     RUN_SINGLE_CASE=$(RUN_SINGLE_CASE) \
	     hack/test.sh'"

ci: all test

clean:
	@echo "cleaning"
	rm -rf .container-* _output/.dockerfile-*
	rm -rf .go _output
	$(DOCKER) rmi $(BUILDER_IMAGE)
