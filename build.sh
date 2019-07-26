#! /bin/bash
# Copyright 2019 Kuei-chun Chen. All rights reserved.
docker-compose down

# build simagix/ftdc
dep ensure -update
env GOOS=linux GOARCH=amd64 go build -o ftdc-linux-x64 simple_json.go
docker build -t simagix/ftdc -f Dockerfile .
docker build -t simagix/ftdc:0.2.0 -f Dockerfile .
rm -f ftdc-linux-x64
mkdir -p ./diagnostic.data/

# build simagix/grafana-ftdc
docker build -t simagix/grafana-ftdc -f grafana/Dockerfile .
docker build -t simagix/grafana-ftdc:0.2.0 -f grafana/Dockerfile .

docker rmi -f $(docker images -f "dangling=true" -q)
