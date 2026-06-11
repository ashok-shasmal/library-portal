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

echo "Starting port-forward to localhost:8080..."
exec kubectl port-forward svc/library-app 8080:8080
