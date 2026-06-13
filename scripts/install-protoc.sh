#!/usr/bin/env bash
set -euo pipefail

# Installs protoc and protoc-gen-go (Linux x86_64). Adjust URLs if needed.
PROTOC_VERSION="23.4"
GO_PLUGIN_VERSION="v1.30.0"

echo "Installing protoc ${PROTOC_VERSION}..."
ARCH="linux-x86_64"
ZIP=protoc-${PROTOC_VERSION}-${ARCH}.zip
URL=https://github.com/protocolbuffers/protobuf/releases/download/v${PROTOC_VERSION}/${ZIP}

tmpdir="$(mktemp -d)"
cd "$tmpdir"

curl -LO "$URL"
if ! command -v unzip >/dev/null 2>&1; then
  echo "unzip is required; installing unzip..."
  sudo apt-get update && sudo apt-get install -y unzip
fi
unzip -o "$ZIP" -d protoc3
sudo mv protoc3/bin/protoc /usr/local/bin/
sudo mv -v protoc3/include/* /usr/local/include/
cd -
rm -rf "$tmpdir"

echo "Installing protoc-gen-go ${GO_PLUGIN_VERSION}..."
GO111MODULE=on go install google.golang.org/protobuf/cmd/protoc-gen-go@${GO_PLUGIN_VERSION}

echo "Done. Make sure $(go env GOPATH)/bin is in your PATH." 
