#! /bin/bash
# Copyright 2019 Kuei-chun Chen. All rights reserved.

DEP=`which dep`
if [ "$DEP" == "" ]; then
    echo "dep command not found"
    exit
fi

if [ -d vendor ]; then
    UPDATE="-update"
fi
export ver=$(cat version)
export version="v${ver}-$(date "+%Y%m%d")"
mkdir -p dist
$DEP ensure $UPDATE
env CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags "-X main.version=$version" -o dist/ftdc-osx-x64 simple_json.go

if [ "$1" == "docker" ]; then
  env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-X main.version=$version" -o ftdc-linux-x64 simple_json.go
  docker-compose down
  docker build -f Dockerfile.local -t simagix/ftdc -t simagix/ftdc:${ver} .
  rm -f ftdc-linux-x64
  mkdir -p ./diagnostic.data/
  docker build -f grafana/Dockerfile -t simagix/grafana-ftdc -t simagix/grafana-ftdc:${ver} .
  docker rmi -f $(docker images -f "dangling=true" -q)
fi
