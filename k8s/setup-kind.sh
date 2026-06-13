#!/usr/bin/env bash
set -euo pipefail

CLUSTER_NAME="demo"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

echo "Cleaning existing kind cluster '$CLUSTER_NAME'..."
kind delete cluster --name "$CLUSTER_NAME" >/dev/null 2>&1 || true

echo "Creating kind cluster '$CLUSTER_NAME'..."
kind create cluster --name "$CLUSTER_NAME"

echo "Building Go app Docker image..."
docker build -t library-app:latest .

echo "Loading image into kind cluster..."
kind load docker-image library-app:latest --name "$CLUSTER_NAME"

echo "Creating PostgreSQL credentials secret..."
POSTGRES_USER="${POSTGRES_USER:-ashok}"
POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-ashok123!}"
POSTGRES_DB="${POSTGRES_DB:-library_db}"

kubectl create secret generic postgres-credentials \
  --from-literal=POSTGRES_USER="$POSTGRES_USER" \
  --from-literal=POSTGRES_PASSWORD="$POSTGRES_PASSWORD" \
  --from-literal=POSTGRES_DB="$POSTGRES_DB" \
  --dry-run=client -o yaml | kubectl apply -f -

echo "Applying Kubernetes manifests..."
kubectl apply -f k8s/postgres-statefulset.yaml
kubectl apply -f k8s/library-app-deployment.yaml

echo "Waiting for PostgreSQL pods to be ready..."
kubectl wait --for=condition=ready pod -l app=postgres --timeout=180s

echo "Waiting for library-app pods to be ready..."
kubectl wait --for=condition=ready pod -l app=library-app --timeout=240s

echo "Starting resilient port-forward to localhost:8080..."
# Ensure the helper script is executable
chmod +x k8s/port-forward.sh
# start the helper with nohup, write pid and log
if [ -f .library-portal-pf.pid ]; then
  oldpid=$(cat .library-portal-pf.pid)
  if ps -p "$oldpid" >/dev/null 2>&1; then
    echo "port-forward helper already running (pid $oldpid); skipping start"
  else
    echo "stale pidfile found; removing"
    rm -f .library-portal-pf.pid
  fi
fi

# if another process is already listening on the port, skip starting
if ss -ltnp 2>/dev/null | grep -q "127.0.0.1:8080"; then
  echo "port 127.0.0.1:8080 already in use; skipping starting helper"
else
  nohup bash k8s/port-forward.sh >/tmp/library-portal-pf.log 2>&1 &
  pf_pid=$!
  echo "$pf_pid" > .library-portal-pf.pid
  echo "resilient port-forward started (pid $pf_pid), log=/tmp/library-portal-pf.log"
  echo "To stop: kill \$(cat .library-portal-pf.pid) && rm .library-portal-pf.pid /tmp/library-portal-pf.log"
fi
echo "Setup complete."