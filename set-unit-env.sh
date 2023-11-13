#!/bin/bash

export VDDK_LIBS=github.com/vmware-tanzu/velero-plugin-for-vsphere/.libs/vmware-vix-disklib-distrib/lib64
export TARGETS=./pkg/...
export TIMEOUT=300s
export VERBOSE=
export DISABLE_CACHE=
export RUN_SINGLE_CASE=controller


