#! /bin/bash
# Copyright 2019-present Kuei-chun Chen. All rights reserved.
die() { echo "$*" 1>&2 ; exit 1; }
[[ "$(which go)" = "" ]] && die "go command not found"
REPO=$(basename "$(dirname "$(pwd)")")/$(basename "$(pwd)")
VERSION="v$(cat version)-$(date "+%Y%m%d")"
LDFLAGS="-X main.version=$VERSION -X main.repo=$REPO"
TAG="simagix/ftdc"
BR=$(git branch --show-current)

mkdir -p dist
if [ "$1" == "docker" ]; then
    docker-compose down > /dev/null 2>&1
    if [[ "${BR}" == "master" ]]; then
        BR="latest"
    fi
    docker build --no-cache -f Dockerfile -t ${TAG}:${BR} .
elif [ "$1" == "grafana" ]; then
    docker build --no-cache -f grafana/Dockerfile -t simagix/grafana-ftdc:${BR} .
else
    env CGO_ENABLED=0 go build -ldflags "$LDFLAGS" -o dist/ftdc_json main/ftdc_json.go
    dist/ftdc_json -version
fi
