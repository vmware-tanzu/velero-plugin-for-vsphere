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
GVDDK:= github.com/vmware/gvddk

#BUILD_IMAGE ?= golang:1.12-stretch

IMAGE ?= velero/velero-plugin-for-vsphere

# Which architecture to build - see $(ALL_ARCH) for options.
# if the 'local' rule is being run, detect the ARCH from 'go env'
# if it wasn't specified by the caller.
local : ARCH ?= $(shell go env GOOS)-$(shell go env GOARCH)
ARCH ?= linux-amd64

VERSION ?= master

platform_temp = $(subst -, ,$(ARCH))
GOOS = $(word 1, $(platform_temp))
GOARCH = $(word 2, $(platform_temp))

# datamgr only begin
BUILDER_IMAGE := vsphere-plugin-builder
DATAMGR_BIN := datamgr
DATAMGR_DOCKERFILE ?= Dockerfile-$(DATAMGR_BIN)
REGISTRY ?= lintongj
DATAMGR_IMAGE = $(REGISTRY)/$(DATAMGR_BIN)
DOTFILE_IMAGE = $(subst :,_,$(subst /,_,$(DATAMGR_IMAGE))-$(VERSION))
# datamgr only end

all: $(addprefix build-, $(BIN))

datamgr:
	$(MAKE) build-datamgr

build-datamgr: build-dirs
	@echo "building: $@"
	$(MAKE) datamgr-shell CMD="-c '\
		GOOS=$(GOOS) \
		GOARCH=$(GOARCH) \
		VERSION=$(VERSION) \
		PKG=$(PKG) \
		BIN=$(DATAMGR_BIN) \
		OUTPUT_DIR=/output/$(GOOS)/$(GOARCH) \
		./hack/build-datamgr.sh'"

datamgr-shell-debug: build-dirs
	@# the volume bind-mount of $PWD/vendor/k8s.io/api is needed for code-gen to
	@# function correctly (ref. https://github.com/kubernetes/kubernetes/pull/64567)
	@docker run \
		-e GOFLAGS \
		-i $(TTY) \
		--rm \
		-u $$(id -u):$$(id -g) \
		-v "$$(pwd)/vendor/k8s.io/api:/go/src/k8s.io/api:delegated" \
		-v "$$(pwd)/.go/pkg:/go/pkg:delegated" \
		-v "$$(pwd)/.go/std:/go/std:delegated" \
		-v "$$(pwd):/go/src/$(PKG):delegated" \
		-v "$$(pwd)/_output/bin:/output:delegated" \
		-v "$$(pwd)/.go/std/$(GOOS)/$(GOARCH):/usr/local/go/pkg/$(GOOS)_$(GOARCH)_static:delegated" \
		-v "$$(pwd)/.go/go-build:/.cache/go-build:delegated" \
		-w /go/src/$(PKG) \
		$(BUILDER_IMAGE) \
		/bin/sh

datamgr-shell: build-dirs build-image
	@# the volume bind-mount of $PWD/vendor/k8s.io/api is needed for code-gen to
	@# function correctly (ref. https://github.com/kubernetes/kubernetes/pull/64567)
	@docker run \
		-e GOFLAGS \
		-i $(TTY) \
		--rm \
		-u $$(id -u):$$(id -g) \
		-v "$$(pwd)/vendor/k8s.io/api:/go/src/k8s.io/api:delegated" \
		-v "$$(pwd)/.go/pkg:/go/pkg:delegated" \
		-v "$$(pwd)/.go/std:/go/std:delegated" \
		-v "$$(pwd):/go/src/$(PKG):delegated" \
		-v "$$(pwd)/_output/bin:/output:delegated" \
		-v "$$(pwd)/.go/std/$(GOOS)/$(GOARCH):/usr/local/go/pkg/$(GOOS)_$(GOARCH)_static:delegated" \
		-v "$$(pwd)/.go/go-build:/.cache/go-build:delegated" \
		-w /go/src/$(PKG) \
		$(BUILDER_IMAGE) \
		/bin/sh $(CMD)

datamgr-container: .container-$(DOTFILE_IMAGE) container-name
.container-$(DOTFILE_IMAGE): build-datamgr $(DATAMGR_DOCKERFILE)
	@cp $(DATAMGR_DOCKERFILE) _output/.dockerfile-$(DATAMGR_BIN)-$(GOOS)-$(GOARCH)
	@docker build --pull -t $(DATAMGR_IMAGE):$(VERSION) -f _output/.dockerfile-$(DATAMGR_BIN)-$(GOOS)-$(GOARCH) _output
	@docker images -q $(DATAMGR_IMAGE):$(VERSION) > $@

container-name:
	@echo "container: $(DATAMGR_IMAGE):$(VERSION)"

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
	$(MAKE) shell

TTY := $(shell tty -s && echo "-t")

shell: build-dirs astrolabe build-image
	@echo "running docker: $@"
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
		-w /go/src/$(PKG) \
		$(BUILDER_IMAGE) \
		go build -installsuffix "static" -i -v -o _output/bin/$(GOOS)/$(GOARCH)/$(BIN) ./$(BIN)

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

container: all
	mkdir -p _output/bin/$(GOOS)/$(GOARCH)/lib/vmware-vix-disklib/lib64
	cp $(GOPATH)/src/$(GVDDK)/lib/vmware-vix-disklib/lib64/* _output/bin/$(GOOS)/$(GOARCH)/lib/vmware-vix-disklib/lib64
	cp Dockerfile _output/bin/$(GOOS)/$(GOARCH)/Dockerfile
	docker build -t $(IMAGE) -f _output/bin/$(GOOS)/$(GOARCH)/Dockerfile _output/bin/$(GOOS)/$(GOARCH)

update:
	@$(MAKE) datamgr-shell CMD="-c 'hack/update-generated-crd-code.sh'"

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
