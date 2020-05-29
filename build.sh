#! /bin/bash
# Copyright 2019 Kuei-chun Chen. All rights reserved.

if [ -d "vendor" ]; then
  dep ensure -update
else
  dep ensure
fi

ver=1.0
version="v${ver}-$(date "+%Y%m%d")"
mkdir -p bin
env CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags "-X main.version=$version" -o bin/ftdc-osx-x64 simple_json.go

if [ "$1" == "docker" ]; then
  env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-X main.version=$version" -o ftdc-linux-x64 simple_json.go
  docker-compose down
  docker build -f Dockerfile.local -t simagix/ftdc -t simagix/ftdc:${ver} .
  rm -f ftdc-linux-x64
  mkdir -p ./diagnostic.data/
  docker build -f grafana/Dockerfile -t simagix/grafana-ftdc -t simagix/grafana-ftdc:${ver} .
  docker rmi -f $(docker images -f "dangling=true" -q)
fi
