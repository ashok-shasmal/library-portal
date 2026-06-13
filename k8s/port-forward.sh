#!/usr/bin/env bash
set -euo pipefail

# Resilient port-forward loop. Keeps forwarding svc/library-app:8080 -> localhost:8080
SERVICE=${SERVICE:-library-app}
LOCAL_PORT=${LOCAL_PORT:-8080}
TARGET_PORT=${TARGET_PORT:-8080}
ADDRESS=${ADDRESS:-127.0.0.1}

echo "starting resilient port-forward for ${SERVICE} -> ${ADDRESS}:${LOCAL_PORT}:${TARGET_PORT}"
# if local port is already listening, exit to avoid multiple forwards
if ss -ltnp 2>/dev/null | grep -q "${ADDRESS}:${LOCAL_PORT}"; then
  echo "local port ${ADDRESS}:${LOCAL_PORT} already in use; exiting"
  exit 0
fi

while true; do
  kubectl port-forward svc/${SERVICE} ${LOCAL_PORT}:${TARGET_PORT} --address ${ADDRESS} || true
  echo "port-forward exited; restarting in 1s..."
  sleep 1
done
