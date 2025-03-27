# Kronicler Developer Resources

## Scope
This page provides links to resources useful to a Kronicler developer.

Kronicler is a utility that stores Kubernetes resources off of the
Kubernetes cluster. This enables users to delete those resources from
the cluster without losing the information contained in those resources.

## Communication
Chat rooms and email addresses for Kronicler communication.

TBD

## Dashboards
TBD

## Code Repositories
Git repository for the Kronicler source code.

* [GitHub Kronicler Repository](https://github.com/kronicler/kronicler)

## Development Resources

### Kubernetes
Kubernetes is an open-source container orchestration engine for automating
the deployment, scaling, and management of containerized applications.

* [Kubernetes](https://kubernetes.io/)

### Programming Languages
The Go programming language is an open-source programming language.
Go is the preferred language for Kronicler development.

* [Go](https://go.dev/)
* [Exercism Learning Track for Go](https://exercism.org/tracks/go)
* [Golang By Example - Advanced Tutorial](https://golangbyexample.com/golang-comprehensive-tutorial/)
* [Ardan Labs Ultimate Go Tour](https://tour.ardanlabs.com/tour/eng/list)
* [Dave Cheney's Practical Go: Real World Advice Guide](https://dave.cheney.net/practical-go/presentations/qcon-china.html)

### Observability
OpenTelemetry is used to instrument, generate, collect, and export
telemetry data (metrics, logs, and traces) to help analyze software’s
performance and behavior.

* [OpenTelemetry](https://opentelemetry.io)
* [OpenTelemetry Go Documentation](https://opentelemetry.io/docs/languages/go/)
* [OpenTelemetry Repository for Go Instrumentation](https://github.com/open-telemetry/opentelemetry-go-instrumentation)

### Tools
Additional software that Kronicler uses.

* [Helm - Package Managment for Kubernetes](https://helm.sh/)
* [Kind - Local Kubernetes Cluster](https://kind.sigs.k8s.io/)
* [Podman Desktop - Includes Kind](https://podman-desktop.io/)

To interact directly with the database:
* psql
    ```bash
    $ sudo dnf install postgresql
    $ psql --version
    psql (PostgreSQL) 15.6
    ```

A SQL client is recommended to interact easily with the database. For example [DBeaver Community](https://dbeaver.io/). To configure DBeaver, check these 
[instructions](https://github.com/kronicler/kronicler/blob/main/database/README.md).

If you use a Professional version of a Jetbrains IDE like `Goland` it already provides a built-in 
[SQL client](https://www.jetbrains.com/help/go/relational-databases.html).

