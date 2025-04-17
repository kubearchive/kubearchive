# Generating the KubeArchive schema file.

Accessing the PostgreSQL database from outside the cluster requires port forwarding. This
command should be run in its own terminal:

```bash
kubectl -n postgresql port-forward service/kubearchive-rw 5433:5432 &
```

The local port (5433) in this case can be changed, but whatever value is chosen
needs to be used in the following command.
