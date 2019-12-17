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

# The binary to build (just the basename).
BIN ?= $(wildcard velero-*)

# This repo's root import path (under GOPATH).
PKG := github.com/vmware-tanzu/velero-plugin-for-vsphere
ASTROLABE:= github.com/vmware-tanzu/astrolabe
#
# The Virtual Disk Development Kit (VDDK) is required for interfacing with vSphere and VADP.
# Please see the gvddk README.md file for instructions on downloading and
# installing.  
# <gopath>/github.com/vmware-tanzu/astrolabe/vendor/github.com/vmware/gvddk/README.md
#
GVDDK:= github.com/vmware-tanzu/astrolabe/vendor/github.com/vmware/gvddk
VDDK_LIBS:= $(GOPATH)/src/$(GVDDK)/vmware-vix-disklib-distrib/lib64

BUILD_IMAGE ?= golang:1.12-stretch

IMAGE ?= velero/velero-plugin-for-vsphere

# Which architecture to build - see $(ALL_ARCH) for options.
# if the 'local' rule is being run, detect the ARCH from 'go env'
# if it wasn't specified by the caller.
local : ARCH ?= $(shell go env GOOS)-$(shell go env GOARCH)
ARCH ?= linux-amd64

platform_temp = $(subst -, ,$(ARCH))
GOOS = $(word 1, $(platform_temp))
GOARCH = $(word 2, $(platform_temp))

all: $(addprefix build-, $(BIN))

build-%:
	$(MAKE) --no-print-directory BIN=$* build

local: build-dirs
	GOOS=$(GOOS) \
	GOARCH=$(GOARCH) \
	PKG=$(PKG) \
	BIN=$(BIN) \
	OUTPUT_DIR=$$(pwd)/_output/bin/$(GOOS)/$(GOARCH) \
	./hack/build.sh

build: _output/bin/$(GOOS)/$(GOARCH)/$(BIN)

_output/bin/$(GOOS)/$(GOARCH)/$(BIN): build-dirs
	@echo "building: $@"
	$(MAKE) shell CMD="-c '\
		GOOS=$(GOOS) \
		GOARCH=$(GOARCH) \
		PKG=$(PKG) \
		BIN=$(BIN) \
		OUTPUT_DIR=/output/$(GOOS)/$(GOARCH) \
		./hack/build.sh'"

TTY := $(shell tty -s && echo "-t")

shell: build-dirs astrolabe
	@echo "running docker: $@"
	docker run \
		-e GOFLAGS \
		-i $(TTY) \
		--rm \
		-u $$(id -u):$$(id -g) \
		-v $$(pwd)/.go/pkg:/go/pkg \
		-v $$(pwd)/.go/src:/go/src \
		-v $$(pwd)/.go/std:/go/std \
		-v $$(pwd):/go/src/$(PKG) \
		-v $$(pwd)/.go/std/$(GOOS)_$(GOARCH):/usr/local/go/pkg/$(GOOS)_$(GOARCH)_static \
		-v "$$(pwd)/.go/go-build:/.cache/go-build:delegated" \
		-e CGO_ENABLED=1 \
		-w /go/src/$(PKG) \
		$(BUILD_IMAGE) \
		go build -installsuffix "static" -i -v -o _output/bin/$(GOOS)/$(GOARCH)/$(BIN) ./$(BIN)

build-dirs:
	@mkdir -p _output/bin/$(GOOS)/$(GOARCH)
	@mkdir -p .go/src/$(PKG) .go/pkg .go/bin .go/std/$(GOOS)/$(GOARCH) .go/go-build

copy-pkgs:
	@echo "copy astrolabe for vendor directory to .go"
	@rm -rf $$(pwd)/.go/src/$(ASTROLABE)
	@mkdir -p $$(pwd)/.go/src/$(ASTROLABE)
	@cp -R $(GOPATH)/src/$(ASTROLABE)/* $$(pwd)/.go/src/$(ASTROLABE)

#	@echo "copy gvddk for vendor directory to .go"
#	@rm -rf $$(pwd)/.go/src/$(GVDDK)
#	mkdir -p $$(pwd)/.go/src/$(GVDDK)
#	@cp -R $(GOPATH)/src/$(GVDDK)/* $$(pwd)/.go/src/$(GVDDK)

astrolabe: build-dirs copy-pkgs
	@echo "building astrolabe"
	docker run \
		-e GOFLAGS \
		-i $(TTY) \
		--rm \
		-u $$(id -u):$$(id -g) \
		-v $$(pwd)/.go/pkg:/go/pkg \
		-v $$(pwd)/.go/src:/go/src \
		-v $$(pwd)/.go/std:/go/std \
		-v $$(pwd):/go/src/$(PKG) \
		-v $$(pwd)/.go/std/$(GOOS)_$(GOARCH):/usr/local/go/pkg/$(GOOS)_$(GOARCH)_static \
		-v "$$(pwd)/.go/go-build:/.cache/go-build:delegated" \
		-e CGO_ENABLED=1 \
		-w /go/src/$(ASTROLABE) \
		$(BUILD_IMAGE) \
		make
VDDK_FILES_TO_COPY = libdiskLibPlugin.so libgvmomi.so libssoclient.so libvim-types.so libvixDiskLib.so libvixDiskLib.so.6 \
libvixDiskLib.so.6.7.0 libvixDiskLibVim.so libvixDiskLibVim.so.6 libvixDiskLibVim.so.6.7.0 libvixMntapi.so libvixMntapi.so.1 \
libvixMntapi.so.1.1.0 libvmacore.so libvmomi.so

container: all
	mkdir -p _output/bin/$(GOOS)/$(GOARCH)/lib/vmware-vix-disklib/lib64
	for lib in $(VDDK_FILES_TO_COPY); do cp -f $(VDDK_LIBS)/$$lib _output/bin/$(GOOS)/$(GOARCH)/lib/vmware-vix-disklib/lib64; done
	cp Dockerfile _output/bin/$(GOOS)/$(GOARCH)/Dockerfile
	docker build -t $(IMAGE) -f _output/bin/$(GOOS)/$(GOARCH)/Dockerfile _output/bin/$(GOOS)/$(GOARCH)

all-ci: $(addprefix ci-, $(BIN))

ci-%:
	$(MAKE) --no-print-directory BIN=$* ci

ci:
	mkdir -p _output
	CGO_ENABLED=1 go build -v -o _output/bin/$(GOOS)/$(GOARCH)/$(BIN) ./$(BIN)

clean:
	@echo "cleaning"
	rm -rf .go _output
