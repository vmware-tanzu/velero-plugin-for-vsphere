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
GVDDK:= github.com/vmware/gvddk


# The binary to build (just the basename).
PLUGIN_BIN ?= $(wildcard velero-*)
DATAMGR_BIN ?= $(wildcard data-manager-*)

REGISTRY ?= vsphereveleroplugin
PLUGIN_IMAGE ?= $(REGISTRY)/$(PLUGIN_BIN)
DATAMGR_IMAGE ?= $(REGISTRY)/$(DATAMGR_BIN)

# Which architecture to build - see $(ALL_ARCH) for options.
# if the 'local' rule is being run, detect the ARCH from 'go env'
# if it wasn't specified by the caller.
local : ARCH ?= $(shell go env GOOS)-$(shell go env GOARCH)
ARCH ?= linux-amd64

VERSION ?= latest

platform_temp = $(subst -, ,$(ARCH))
GOOS = $(word 1, $(platform_temp))
GOARCH = $(word 2, $(platform_temp))

BUILDER_IMAGE := vsphere-plugin-builder
PLUGIN_DOCKERFILE ?= Dockerfile-plugin
DATAMGR_DOCKERFILE ?= Dockerfile-datamgr

all: plugin

plugin: datamgr
	@echo "making: $@"
	$(MAKE) build BIN=$(PLUGIN_BIN)

datamgr: astrolabe
	@echo "making: $@"
	$(MAKE) build BIN=$(DATAMGR_BIN)

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
		REGISTRY=$(REGISTRY) \
		VERSION=$(VERSION) \
		PKG=$(PKG) \
		BIN=$(BIN) \
		OUTPUT_DIR=/output/$(GOOS)/$(GOARCH) \
		./hack/build.sh'"

TTY := $(shell tty -s && echo "-t")

shell: build-dirs build-image
	@echo "running docker: $@"
	docker run \
		-e GOFLAGS \
		-i $(TTY) \
		--rm \
		-u $$(id -u):$$(id -g) \
		-v "$$(pwd)/vendor/k8s.io/api:/go/src/k8s.io/api:delegated" \
		-v $$(pwd)/.go/pkg:/go/pkg:delegated \
		-v $$(pwd)/.go/src:/go/src:delegated \
		-v $$(pwd)/.go/std:/go/std:delegated \
		-v $$(pwd):/go/src/$(PKG):delegated \
		-v "$$(pwd)/_output/bin:/output:delegated" \
		-v $$(pwd)/.go/std/$(GOOS)_$(GOARCH):/usr/local/go/pkg/$(GOOS)_$(GOARCH)_static:delegated \
		-v "$$(pwd)/.go/go-build:/.cache/go-build:delegated" \
		-e CGO_ENABLED=1 \
		-w /go/src/$(PKG) \
		$(BUILDER_IMAGE) \
		/bin/sh $(CMD)

build-dirs:
	@mkdir -p _output/bin/$(GOOS)/$(GOARCH)
	@mkdir -p .go/src/$(PKG) .go/pkg .go/bin .go/std/$(GOOS)/$(GOARCH) .go/go-build

build-image:
	cd hack/build-image && docker build --pull -t $(BUILDER_IMAGE) .

copy-pkgs:
	@echo "copy astrolabe for vendor directory to .go"
	@rm -rf $$(pwd)/.go/src/$(ASTROLABE)
	@mkdir -p $$(pwd)/.go/src/$(ASTROLABE)
	@cp -R $(GOPATH)/src/$(ASTROLABE)/* $$(pwd)/.go/src/$(ASTROLABE)

	@echo "copy gvddk for vendor directory to .go"
	@rm -rf $$(pwd)/.go/src/$(GVDDK)
	mkdir -p $$(pwd)/.go/src/$(GVDDK)
	@cp -R $(GOPATH)/src/$(GVDDK)/* $$(pwd)/.go/src/$(GVDDK)

astrolabe: build-dirs copy-pkgs build-image
	@echo "building astrolabe"
	docker run \
		-e GOFLAGS \
		-i $(TTY) \
		--rm \
		-u $$(id -u):$$(id -g) \
		-v $$(pwd)/.go/pkg:/go/pkg:delegated \
		-v $$(pwd)/.go/src:/go/src:delegated \
		-v $$(pwd)/.go/std:/go/std:delegated \
		-v $$(pwd):/go/src/$(PKG):delegated \
		-v $$(pwd)/.go/std/$(GOOS)_$(GOARCH):/usr/local/go/pkg/$(GOOS)_$(GOARCH)_static:delegated \
		-v "$$(pwd)/.go/go-build:/.cache/go-build:delegated" \
		-e CGO_ENABLED=1 \
		-w /go/src/$(ASTROLABE) \
		$(BUILDER_IMAGE) \
		make

container-name:
	@echo "container: $(IMAGE):$(VERSION)"

copy-vix-libs:
	mkdir -p _output/bin/$(GOOS)/$(GOARCH)/lib/vmware-vix-disklib/lib64
	cp $$(pwd)/.go/src/$(GVDDK)/lib/vmware-vix-disklib/lib64/* _output/bin/$(GOOS)/$(GOARCH)/lib/vmware-vix-disklib/lib64

copy-install-script:
	cp $$(pwd)/scripts/install.sh _output/bin/$(GOOS)/$(GOARCH)

build-container: copy-vix-libs container-name
	cp $(DOCKERFILE) _output/bin/$(GOOS)/$(GOARCH)/$(DOCKERFILE)
	docker build -t $(IMAGE):$(VERSION) -f _output/bin/$(GOOS)/$(GOARCH)/$(DOCKERFILE) _output

plugin-container: all copy-install-script
	$(MAKE) build-container IMAGE=$(PLUGIN_IMAGE) DOCKERFILE=$(PLUGIN_DOCKERFILE)

datamgr-container: datamgr
	$(MAKE) build-container BIN=$(DATAMGR_BIN) IMAGE=$(DATAMGR_IMAGE) DOCKERFILE=$(DATAMGR_DOCKERFILE)

container: plugin-container datamgr-container

update:
	@echo "updating CRDs"
	./hack/update-generated-crd-code.sh

push-plugin: plugin-container
	docker push $(PLUGIN_IMAGE):$(VERSION)

push-datamgr: datamgr-container
	docker push $(DATAMGR_IMAGE):$(VERSION)

push: push-datamgr push-plugin

all-ci: $(addprefix ci-, $(BIN))

ci-%:
	$(MAKE) --no-print-directory BIN=$* ci

ci:
	mkdir -p _output
	CGO_ENABLED=1 go build -v -o _output/bin/$(GOOS)/$(GOARCH)/$(BIN) ./$(BIN)

clean:
	@echo "cleaning"
	rm -rf .container-* _output/.dockerfile-*
	rm -rf .go _output
	docker rmi $(BUILDER_IMAGE)
