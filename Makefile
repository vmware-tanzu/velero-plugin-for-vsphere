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
ASTROLABE:= github.com/vmware-tanzu/astrolabe


# The Virtual Disk Development Kit (VDDK) is required for interfacing with vSphere and VADP.
# Please refer to https://github.com/vmware/virtual-disks#dependency for the dependency.
# To compile plugin, we need to download the VDDK tarball
# under $(PKG)/.libs directory (create the directory if it unexists)
LIB_DIR := $(PKG)/.libs
VDDK_LIBS:= $(LIB_DIR)/vmware-vix-disklib-distrib/lib64

# The binary to build (just the basename).
PLUGIN_BIN ?= velero-plugin-for-vsphere
DATAMGR_BIN ?= data-manager-for-plugin
BACKUPDRIVER_BIN ?= backup-driver
VSPHERE_ASTROLABE ?= vsphere-astrolabe

RELEASE_REGISTRY = ?vsphereveleroplugin
REGISTRY ?= dpcpinternal
PLUGIN_IMAGE ?= $(REGISTRY)/$(PLUGIN_BIN)
DATAMGR_IMAGE ?= $(REGISTRY)/$(DATAMGR_BIN)
BACKUPDRIVER_IMAGE ?= $(REGISTRY)/$(BACKUPDRIVER_BIN)

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

BUILDER_IMAGE := golang:1.18
PLUGIN_DOCKERFILE ?= Dockerfile-plugin
DATAMGR_DOCKERFILE ?= Dockerfile-datamgr
BACKUPDRIVER_DOCKERFILE ?= Dockerfile-backup-driver

all: dep plugin vsphere-astrolabe

dep:
ifeq (,$(wildcard $(GOPATH)/src/$(VDDK_LIBS)))
	$(error "$(GOPATH)/src/$(VDDK_LIBS) cannot find vddk libs in path. Please refer to: https://github.com/vmware/virtual-disks#dependency, download the VDDK tarball to the directory $(GOPATH)/src/$(LIB_DIR)/ and untar it")
endif

plugin: datamgr backup-driver
	@echo "making: $@"
	$(MAKE) build BIN=$(PLUGIN_BIN) VERSION=$(VERSION)

datamgr:
	@echo "making: $@"
	$(MAKE) build BIN=$(DATAMGR_BIN) VERSION=$(VERSION)

backup-driver:
	@echo "making: $@"
	$(MAKE) build BIN=$(BACKUPDRIVER_BIN) VERSION=$(VERSION)

vsphere-astrolabe:
	@echo "making: $@"
	$(MAKE) build BIN=$(VSPHERE_ASTROLABE) VERSION=$(VERSION)

local: build-dirs
	GOOS=$(GOOS) \
	GOARCH=$(GOARCH) \
	PKG=$(PKG) \
	BIN=$(BIN) \
	GIT_SHA=$(GIT_SHA) \
	GIT_DIRTY="$(GIT_DIRTY)" \
	OUTPUT_DIR=$$(pwd)/_output/bin/$(GOOS)/$(GOARCH) \
	GO111MODULE=on \
	GOFLAGS=-mod=readonly \ 
	./hack/build.sh

build: _output/bin/$(GOOS)/$(GOARCH)/$(BIN)

_output/bin/$(GOOS)/$(GOARCH)/$(BIN): build-dirs copy-astrolabe
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
		-v $$(pwd)/.libs/vmware-vix-disklib-distrib:/usr/local/vmware-vix-disklib-distrib:delegated \
		-v $$(pwd)/.go/pkg:/go/pkg:delegated \
		-v $$(pwd)/.go/src:/go/src:delegated \
		-v $$(pwd)/.go/std:/go/std:delegated \
		-v $$(pwd):/go/src/$(PKG):delegated \
		-v "$$(pwd)/_output/bin:/output:delegated" \
		-v $$(pwd)/.go/std/$(GOOS)/$(GOARCH):/usr/local/go/pkg/$(GOOS)/$(GOARCH)_static:delegated \
		-v "$$(pwd)/.go/go-build:/.cache/go-build:delegated" \
		-e CGO_ENABLED=1 \
		-e GOPATH=/go \
		-w /go/src/$(PKG) \
		$(BUILDER_IMAGE) \
		/bin/sh $(CMD)

build-dirs:
	@mkdir -p _output/bin/$(GOOS)/$(GOARCH)
	@mkdir -p .go/src/$(PKG) .go/pkg .go/bin .go/std/$(GOOS)/$(GOARCH) .go/go-build

copy-astrolabe:
ifeq (,$(wildcard $(GOPATH)/src/$(ASTROLABE)))
	@echo "skip copying astrolabe as it is not available at $(GOPATH)/src/$(ASTROLABE)"
else
	@echo "copy astrolabe to .go/src directory"
	@rm -rf $$(pwd)/.go/src/$(ASTROLABE)
	@mkdir -p $$(pwd)/.go/src/$(ASTROLABE)
	@cp -R $(GOPATH)/src/$(ASTROLABE)/* $$(pwd)/.go/src/$(ASTROLABE)
endif


container-name:
	@echo "container: $(IMAGE):$(VERSION)"

copy-vix-libs:
	mkdir -p _output/bin/$(GOOS)/$(GOARCH)/lib/vmware-vix-disklib/lib64
	cp -R $(GOPATH)/src/$(VDDK_LIBS)/* _output/bin/$(GOOS)/$(GOARCH)/lib/vmware-vix-disklib/lib64
	# Some of the libraries have the executable bit set and this causes plugin startup to fail
	chmod 644 _output/bin/$(GOOS)/$(GOARCH)/lib/vmware-vix-disklib/lib64/*

copy-install-script:
	cp $$(pwd)/scripts/install.sh _output/bin/$(GOOS)/$(GOARCH)

build-container: copy-vix-libs container-name
	cp $(DOCKERFILE) _output/bin/$(GOOS)/$(GOARCH)/$(DOCKERFILE)
	$(DOCKER) build --platform $(GOOS)/$(GOARCH) -t $(IMAGE):$(VERSION) -f _output/bin/$(GOOS)/$(GOARCH)/$(DOCKERFILE) _output

plugin-container: all copy-install-script
	$(MAKE) build-container IMAGE=$(PLUGIN_IMAGE) DOCKERFILE=$(PLUGIN_DOCKERFILE) VERSION=$(VERSION)

datamgr-container: datamgr
	$(MAKE) build-container BIN=$(DATAMGR_BIN) IMAGE=$(DATAMGR_IMAGE) DOCKERFILE=$(DATAMGR_DOCKERFILE) VERSION=$(VERSION)

backup-driver-container: backup-driver
	$(MAKE) build-container BIN=$(BACKUPDRIVER_BIN) IMAGE=$(BACKUPDRIVER_IMAGE) DOCKERFILE=$(BACKUPDRIVER_DOCKERFILE) VERSION=$(VERSION)

container: plugin-container datamgr-container backup-driver-container

update:
	@echo "updating CRDs"
	./hack/update-generated-crd-code.sh

push-plugin: plugin-container
	$(DOCKER) push $(PLUGIN_IMAGE):$(VERSION)

push-datamgr: datamgr-container
	$(DOCKER) push $(DATAMGR_IMAGE):$(VERSION)

push-backup-driver: backup-driver-container
	$(DOCKER) push $(BACKUPDRIVER_IMAGE):$(VERSION)

push: push-datamgr push-plugin push-backup-driver

QUALIFIED_TAG ?=
RELEASE_TAG ?= latest
release:
ifneq (,$(QUALIFIED_TAG))
	$(DOCKER) pull $(DATAMGR_IMAGE):$(QUALIFIED_TAG)
	$(DOCKER) pull $(PLUGIN_IMAGE):$(QUALIFIED_TAG)
	$(DOCKER) pull $(BACKUPDRIVER_IMAGE):$(QUALIFIED_TAG)
	$(DOCKER) tag $(BACKUPDRIVER_IMAGE):$(QUALIFIED_TAG) $(RELEASE_REGISTRY)/$(BACKUPDRIVER_BIN):$(RELEASE_TAG)
	$(DOCKER) tag $(DATAMGR_IMAGE):$(QUALIFIED_TAG) $(RELEASE_REGISTRY)/$(DATAMGR_BIN):$(RELEASE_TAG)
	$(DOCKER) tag $(PLUGIN_IMAGE):$(QUALIFIED_TAG) $(RELEASE_REGISTRY)/$(PLUGIN_BIN):$(RELEASE_TAG)
	$(DOCKER) push $(RELEASE_REGISTRY)/$(BACKUPDRIVER_BIN):$(RELEASE_TAG)
	$(DOCKER) push $(RELEASE_REGISTRY)/$(DATAMGR_BIN):$(RELEASE_TAG)
	$(DOCKER) push $(RELEASE_REGISTRY)/$(PLUGIN_BIN):$(RELEASE_TAG)
endif

verify:
	@echo "verify: Started"
	@echo "verify: Completed"

TARGETS ?= ./pkg/...
TIMEOUT ?= 300s
VERBOSE ?= # empty by default
DISABLE_CACHE ?= # empty by default
RUN_SINGLE_CASE ?= # empty by default
test: build-dirs
	@$(MAKE) shell CMD="-c '\
	     VDDK_LIBS=$(VDDK_LIBS) \
	     TARGETS=$(TARGETS) \
	     TIMEOUT=$(TIMEOUT) \
	     VERBOSE=$(VERBOSE) \
	     DISABLE_CACHE=$(DISABLE_CACHE) \
	     RUN_SINGLE_CASE=$(RUN_SINGLE_CASE) \
	     hack/test.sh'"

ci: all verify test

clean:
	@echo "cleaning"
	rm -rf .container-* _output/.dockerfile-*
	rm -rf .go _output
	$(DOCKER) rmi $(BUILDER_IMAGE)
