#!/usr/bin/env sh
set -e

# Needed, as operator-sdk does not handle GOROOT correctly
GOROOT=$(go env GOROOT)
export GOROOT

operator-sdk generate k8s
operator-sdk generate crds
