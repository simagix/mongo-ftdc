#! /bin/bash
# Copyright 2019 Kuei-chun Chen. All rights reserved.
docker-compose down
docker build -t simagix/grafana-ftdc -f Dockerfile .
docker build -t simagix/grafana-ftdc:1.0 -f Dockerfile .
docker rmi -f $(docker images -f "dangling=true" -q)
mkdir -p ./diagnostic.data/

