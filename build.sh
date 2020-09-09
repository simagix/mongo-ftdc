#! /bin/bash
# Copyright 2019 Kuei-chun Chen. All rights reserved.

if [[ "$(which go)" == "" ]]; then
  echo "go command not found"
  exit
fi

DEP=`which dep`
if [[ "$DEP" == "" ]]; then
    echo "dep command not found"
    exit
fi

if [[ -d vendor ]]; then
    UPDATE="-update"
fi
export ver=$(cat version)
export version="v${ver}-$(date "+%Y%m%d")"
mkdir -p dist
$DEP ensure $UPDATE
env CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags "-X main.version=$version" -o dist/mftdc simple_json.go
dist/mftdc -version
