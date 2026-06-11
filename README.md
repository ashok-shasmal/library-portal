# Library Portal

A simple library management application in Go.

## Local Docker Compose Setup

The `docker-compose.yml` file starts two services:

- `postgres_db`: PostgreSQL 16 database
- `go_app`: Go web application running on port `8080`

Example service configuration:

```yaml
services:
  postgres_db:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: ashok
      POSTGRES_PASSWORD: ashok123!
      POSTGRES_DB: library_db
    ports:
      - "5432:5432"
    volumes:
      - pgdata:/var/lib/postgresql/data

  go_app:
    build: .
    ports:
      - "8080:8080"
    depends_on:
      postgres_db:
        condition: service_healthy
    environment:
      - DB_HOST=postgres_db
      - DB_PORT=5432
      - DB_USER=ashok
      - DB_PASSWORD=ashok123!
      - DB_NAME=library_db

volumes:
  pgdata:
```

Run locally with:

```bash
docker compose up --build
```

## Kubernetes Deployment

The `k8s/` directory contains manifests for deployment to Kubernetes.

### PostgreSQL StatefulSet

File: `k8s/postgres-statefulset.yaml`

- `Service` for PostgreSQL on port `5432`
- headless service for stable pod DNS
- `StatefulSet` with `replicas: 2`
- persistent volume claim template for database storage

### Library App Deployment

File: `k8s/library-app-deployment.yaml`

- `Service` exposing the application on port `8080`
- `Deployment` with `replicas: 2`
- environment variables configured to connect to PostgreSQL
- pod anti-affinity to spread app replicas across nodes

### Deploy to kind

The `k8s/` directory includes:

- `k8s/postgres-secret.yaml` (sample template only)
- `k8s/postgres-statefulset.yaml`
- `k8s/library-app-deployment.yaml`
- `k8s/setup-kind.sh`

Build the app image and load it into the `kind` cluster:

```bash
cd /home/ashok/projects/go/library-portal
docker build -t library-app:latest .
kind load docker-image library-app:latest --name demo
```

Create the PostgreSQL secret and apply the manifests:

```bash
# optional: override defaults with env vars
POSTGRES_USER=ashok \
POSTGRES_PASSWORD=ashok123! \
POSTGRES_DB=library_db \
./k8s/setup-kind.sh
```

Or run the script directly and let it create the secret for you:

```bash
chmod +x k8s/setup-kind.sh
./k8s/setup-kind.sh
```

Check pod and service status:

```bash
kubectl get pods
kubectl get svc
```

Forward the service locally:

```bash
kubectl port-forward svc/library-app 8080:8080
```

Open `http://localhost:8080`.

### Automated kind setup

To clean any existing `demo` cluster, create the cluster, build and load the image, apply manifests, and forward the service:

```bash
chmod +x k8s/setup-kind.sh
./k8s/setup-kind.sh
```

This script will:

- delete any existing `kind` cluster named `demo`
- create a fresh `demo` cluster
- build the `library-app:latest` Docker image
- load that image into the cluster
- apply the secret, PostgreSQL StatefulSet, and app Deployment
- wait for pods to become ready
- forward `svc/library-app` to `http://localhost:8080`

## Notes

- Kubernetes networking does not require a separate network definition like Docker Compose.
- For real leader election or a single active app pod, additional application-level logic or a leader election controller is required.
- The `kind` cluster is lightweight and suitable for local development.
