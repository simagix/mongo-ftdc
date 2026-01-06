#!/bin/bash
# Copyright 2019-present Kuei-chun Chen. All rights reserved.
die() { echo "$*" 1>&2 ; exit 1; }

[[ "$(which go)" = "" ]] && die "go command not found"

EXEC=mftdc
GIT_DATE=$(git log -1 --date=format:"%Y%m%d" --format="%ad" 2>/dev/null || date +"%Y%m%d")
VERSION="v$(cat version)-${GIT_DATE}"
LDFLAGS="-X main.version=$VERSION -X main.repo=$EXEC"
TAG="simagix/ftdc"

print_usage() {
  echo "Usage: $0 [command]"
  echo ""
  echo "Commands:"
  echo "  (none)        Build binary for current platform to dist/"
  echo "  docker        Build Docker image for current platform (local)"
  echo "  push          Build and push multi-arch Docker image (amd64 + arm64)"
  echo "  binaries      Build binaries for all platforms (linux/mac/win, amd64/arm64)"
  echo ""
  echo "Internal (used by Dockerfile):"
  echo "  binary        Build binary for current platform (auto-detected)"
  echo ""
}

mkdir -p dist

case "$1" in
  docker)
    # Build for current platform only (local image for testing)
    BR=$(git branch --show-current)
    if [[ "${BR}" == "main" ]]; then
      BR=$(cat version)
    fi
    docker buildx build --load \
      -t ${TAG}:${BR} \
      -t ${TAG}:latest . || die "docker build failed"
    echo "Built ${TAG}:${BR} for $(uname -m)"
    ;;

  push)
    # Requires: docker buildx create --name multibuilder --driver docker-container --use
    BR=$(git branch --show-current)
    if [[ "${BR}" == "main" ]]; then
      BR=$(cat version)
    fi
    echo "Building multi-arch image for linux/amd64 and linux/arm64..."
    docker buildx build --builder multibuilder --platform linux/amd64,linux/arm64 \
      --provenance=false --sbom=false \
      -t ${TAG}:${BR} \
      -t ${TAG}:latest \
      --push . || die "docker build failed"
    echo "Pushed ${TAG}:${BR} (amd64 + arm64)"
    ;;

  binary)
    # Internal: called by Dockerfile. Builds for current arch (set by docker buildx)
    LDFLAGS="${LDFLAGS} -X main.docker=docker"
    env CGO_ENABLED=0 go build -ldflags "$LDFLAGS" -o $EXEC main/mftdc.go
    echo "Built $EXEC for $(go env GOOS)/$(go env GOARCH)"
    ;;

  binaries)
    echo "Building binaries for all platforms..."
    
    # Linux amd64
    env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "$LDFLAGS" -o dist/${EXEC}-linux-amd64 main/mftdc.go
    echo "  Built dist/${EXEC}-linux-amd64"
    
    # Linux arm64
    env CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags "$LDFLAGS" -o dist/${EXEC}-linux-arm64 main/mftdc.go
    echo "  Built dist/${EXEC}-linux-arm64"
    
    # macOS amd64
    env CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags "$LDFLAGS" -o dist/${EXEC}-darwin-amd64 main/mftdc.go
    echo "  Built dist/${EXEC}-darwin-amd64"
    
    # macOS arm64 (Apple Silicon)
    env CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags "$LDFLAGS" -o dist/${EXEC}-darwin-arm64 main/mftdc.go
    echo "  Built dist/${EXEC}-darwin-arm64"
    
    # Windows amd64
    env CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags "$LDFLAGS" -o dist/${EXEC}-windows-amd64.exe main/mftdc.go
    echo "  Built dist/${EXEC}-windows-amd64.exe"
    
    echo "Done! Binaries in dist/"
    ;;

  help|-h|--help)
    print_usage
    ;;

  "")
    rm -f dist/$EXEC
    env CGO_ENABLED=0 go build -ldflags "$LDFLAGS" -o dist/$EXEC main/mftdc.go
    if [[ -f dist/$EXEC ]]; then
      ./dist/$EXEC -version
    fi
    ;;

  *)
    echo "Unknown command: $1"
    print_usage
    exit 1
    ;;
esac
