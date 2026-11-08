#!/bin/bash
set -e

WORK_DIR=`cd $(dirname "$0")/../..;pwd`

cd $WORK_DIR

mkdir -p external/package

envoy_repos=("go-control-plane" "envoy")

for repo in ${envoy_repos[@]}; do
    if [ -e external/$repo ];then
        continue
    fi
    cp -RP envoy/$repo  external/$repo
    cd external/$repo
    echo "gitdir: /parent/.git/modules/envoy/$repo" > .git
    cd $WORK_DIR
done

istio_repos=("api" "client-go" "pkg" "istio" "proxy")

for repo in ${istio_repos[@]}; do
    if [ -e external/$repo ];then
        continue
    fi
    cp -RP istio/$repo external/$repo
    cd external/$repo
    echo "gitdir: /parent/.git/modules/istio/$repo" > .git
    cd $WORK_DIR
done

# Update root go.mod after all external dependencies are ready
go mod tidy
