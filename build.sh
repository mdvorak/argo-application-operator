#!/usr/bin/env sh

go build -o ./build/_output/bin/argo-application-operator -gcflags all=-trimpath=/home/cen38289/Workspace -asmflags all=-trimpath=/home/cen38289/Workspace github.com/mdvorak/argo-application-operator/cmd/manager
