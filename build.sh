#! /bin/bash
# Copyright 2019 Kuei-chun Chen. All rights reserved.

# build simagix/ftdc
dep ensure -update
ver=0.3.1
mkdir -p bin
env GOOS=darwin GOARCH=amd64 go build -o bin/ftdc-osx-x64 simple_json.go

# build simagix/grafana-ftdc
if [ "$1" == "docker" ]; then
    env GOOS=linux GOARCH=amd64 go build -o ftdc-linux-x64 simple_json.go
    docker build -f Dockerfile -t simagix/ftdc -t simagix/ftdc:${ver} .
    rm -f ftdc-linux-x64
    mkdir -p ./diagnostic.data/
    docker-compose down
    docker build -f grafana/Dockerfile -t simagix/grafana-ftdc -t simagix/grafana-ftdc:${ver} .
    docker rmi -f $(docker images -f "dangling=true" -q)
fi
