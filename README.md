# kronicler

## Overview
Kronicler is a utility that stores Kubernetes resources off of the
Kubernetes cluster. This enables users to delete those resources from
the cluster without losing the information contained in those resources.
Kronicler will provide an API so users can retrieve stored resources
for inspection.

The main users of Kronicler are projects that use Kubernetes resources
for one-shot operations and want to inspect those resources in the long-term.
For example, users using Jobs on Kubernetes that want to track the success
rate over time, but need to remove completed Jobs for performance/storage
reasons, would benefit from Kronicler. Another example would be users
that run build systems on top of Kubernetes (Shipwright, Tekton) that use
resources for one-shot builds and want to keep track of those builds over time.

* [Code of Conduct](./CODE_OF_CONDUCT.md)
* [Documentation](https://kronicler.github.io/kronicler/main/index.html)
* [Kronicler's Contribution Guide](https://kronicler.github.io/kronicler/main/contributors/guide.html)

## Get In Touch

You can get in touch with the Kronicler team at:

* [Slack](https://kubernetes.slack.com/archives/C07MB5YBVCL) to get an invite go [here](https://slack.k8s.io/)
* [Kronicler mailing list](https://groups.google.com/g/kronicler)
* [Kronicler Developers mailing list](https://groups.google.com/g/kronicler-developers)
