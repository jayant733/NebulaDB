# Kubernetes (optional)

These manifests were not applied in CI. They are the Phase 13 contract.

```bash
docker build -t nebuladb:local .
# kind create cluster && kind load docker-image nebuladb:local
kubectl apply -f deploy/k8s/nebuladb.yaml
kubectl -n nebuladb rollout restart statefulset/nebuladb
```

Rolling restart keeps the PVC. Committed `Set` with default fsync is on disk after SIGTERM `Engine.Close`. This is not a measured HA SLA.
