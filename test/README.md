# Integration Tests

## Prerequisites

* Install the normal Kubearchive development tools, see the [README](../README.md).
* Install [ko](https://ko.build/install/).

## Run the tests

```
kind create cluster
go test -v ./test/... -tags=integration
```
