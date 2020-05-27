#! /bin/bash
# Copyright 2019 Kuei-chun Chen. All rights reserved.

if [ -d "vendor" ]; then
  dep ensure -update
else
  dep ensure
fi

ver=1.0
mkdir -p bin
env GOOS=darwin GOARCH=amd64 go build -o bin/ftdc-osx-x64 simple_json.go
env GOOS=linux GOARCH=amd64 go build -o ftdc-linux-x64 simple_json.go
docker-compose down
docker build -f Dockerfile.local -t simagix/ftdc -t simagix/ftdc:${ver} .
rm -f ftdc-linux-x64
mkdir -p ./diagnostic.data/
docker build -f grafana/Dockerfile -t simagix/grafana-ftdc -t simagix/grafana-ftdc:${ver} .
docker rmi -f $(docker images -f "dangling=true" -q)
