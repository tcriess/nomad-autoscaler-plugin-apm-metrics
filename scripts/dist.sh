#!/bin/bash

suffix=""
if [ $1 == "windows" ]; then
  suffix=".exe"
fi

CGO_ENABLED=0 GOOS=$1 GOARCH=$2 go build -ldflags "-s -w" -a -installsuffix cgo -o "dist/metrics${suffix}"
zip -j dist/metrics_$1_$2.zip "dist/metrics${suffix}"
rm -rf "dist/metrics${suffix}"
