# Performance Tests

To run the performance tests, execute the following from the root of the repository:

```bash
kind create cluster
bash hack/quick-install.sh
bash test/performance/run.sh
```

**Note**: this requires at least Python 3.10

The tests are run with [Locust](https://docs.locust.io/en/stable/) in order and they
are the following:

1. `create`: (Sink) POST / to create Pods from a template, aprox ~3k Pods are inserted
1. `get`: (API) GET /api/v1/pods

The results of the tests are on `./perf-results/`, relative to the root of the repository.
Their name reference the test where they come from, currently `get-*.csv` and `create-*.csv`

-   `.txt` files contain summaries of Locust, the time unit is milliseconds.
-   `.csv` files contain values from Prometheus, the CPU unit is in milliCPU, the memory unit is bytes.
