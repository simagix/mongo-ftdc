#! /bin/bash
# Copyright 2019-present Kuei-chun Chen. All rights reserved.
die() { echo "$*" 1>&2 ; exit 1; }
[[ "$(which go)" = "" ]] && die "go command not found"
EXEC=ftdc_json
VERSION="v$(cat version)-$(git log -1 --date=format:"%Y%m%d" --format="%ad")"
LDFLAGS="-X main.version=$VERSION -X main.repo=$EXEC"

mkdir -p dist
if [ "$1" == "docker" ]; then
	VER=$(cat version)
	TAG="ajithkn716/mongo-ftdc"
    docker-compose down > /dev/null 2>&1
    if [[ "${VER}" == "master" ]]; then
        VER="latest"
    fi
    docker build --no-cache -f Dockerfile -t ${TAG}:${VER} .
	docker tag ${TAG}:${VER} ${TAG}

    docker build --no-cache -f grafana/Dockerfile -t ajithkn716/grafana-ftdc:${VER} .
    docker tag ajithkn716/grafana-ftdc:${VER} ajithkn716/grafana-ftdc
else
    env CGO_ENABLED=0 go build -ldflags "$LDFLAGS" -o dist/$EXEC main/ftdc_json.go
    dist/$EXEC -version
fi
