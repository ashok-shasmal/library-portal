SHELL := /bin/bash

.PHONY: dev-up dev-down dev-status pf-start pf-stop dev-build

# build the code locally
dev-build:
	@echo build the repo
	@go build ./cmd/server/main.go 
	@echo dev-build finished

# Create the kind cluster, build image, deploy manifests and start port-forward
dev-up:
	@echo "Starting dev environment..."
	@./k8s/setup-kind.sh
	@echo "dev-up finished"

# Tear down resources and delete the kind cluster
dev-down:
	@echo "Stopping dev environment..."
	-@kubectl delete -f k8s/library-app-deployment.yaml || true
	-@kubectl delete -f k8s/postgres-statefulset.yaml || true
	-@kubectl delete secret postgres-credentials || true
	-@kind delete cluster --name demo || true
	-@rm -f .library-portal-pf.pid /tmp/library-portal-pf.log || true
	@echo "dev-down finished"

# Show cluster/pod status and local port-forward status
dev-status:
	@kubectl get pods -o wide || true
	@kubectl get svc library-app || true
	@ss -ltnp | grep ':8080' || true

# Start the resilient port-forward helper (background)
pf-start:
	@mkdir -p /tmp
	@chmod +x k8s/port-forward.sh
	@if [ -f .library-portal-pf.pid ] && ps -p $$(cat .library-portal-pf.pid) >/dev/null 2>&1; then \
		 echo "port-forward helper already running (pid=$$(cat .library-portal-pf.pid))"; \
	else \
		nohup bash k8s/port-forward.sh >/tmp/library-portal-pf.log 2>&1 & echo $$! > .library-portal-pf.pid && echo "started port-forward (pid=$$(cat .library-portal-pf.pid))"; \
	fi

# Stop the port-forward helper
pf-stop:
	@if [ -f .library-portal-pf.pid ]; then \
		kill $$(cat .library-portal-pf.pid) >/dev/null 2>&1 || true; \
		rm -f .library-portal-pf.pid /tmp/library-portal-pf.log; \
		echo "stopped port-forward"; \
	else \
		echo "no port-forward pidfile found"; \
	fi
# Makefile for protobuf generation
PROTO_DIR := proto
OUT_DIR := internal/pb
PROTO_FILES := $(wildcard $(PROTO_DIR)/*.proto)

.PHONY: proto
proto:
	@if ! command -v protoc >/dev/null 2>&1; then \
		echo "protoc is not installed. See scripts/install-protoc.sh"; exit 1; \
	fi
	protoc -I $(PROTO_DIR) --go_out=paths=source_relative:$(OUT_DIR) $(PROTO_FILES)

.PHONY: gen
gen: proto

.PHONY: test
test:
	go test ./...
