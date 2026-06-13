#!/usr/bin/env bash
set -euo pipefail

PROTO_DIR="$(cd "$(dirname "$0")/.." && pwd)/proto"
OUT_DIR="$(cd "$(dirname "$0")/.." && pwd)/internal/pb"

if ! command -v protoc >/dev/null 2>&1; then
  echo "protoc is not installed. Run scripts/install-protoc.sh or install protoc manually."
  exit 1
fi

protoc -I "${PROTO_DIR}" --go_out=paths=source_relative:${OUT_DIR} ${PROTO_DIR}/*.proto

echo "Generated proto Go code into ${OUT_DIR}"
