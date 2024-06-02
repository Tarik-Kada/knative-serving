# Knative Serving

[![go.dev reference](https://img.shields.io/badge/go.dev-reference-007d9c?logo=go&logoColor=white)](https://pkg.go.dev/github.com/knative/serving)
[![Go Report Card](https://goreportcard.com/badge/knative/serving)](https://goreportcard.com/report/knative/serving)
[![Releases](https://img.shields.io/github/release-pre/knative/serving.svg?sort=semver)](https://github.com/knative/serving/releases)
[![LICENSE](https://img.shields.io/github/license/knative/serving.svg)](https://github.com/knative/serving/blob/main/LICENSE)
[![Slack Status](https://img.shields.io/badge/slack-join_chat-white.svg?logo=slack&style=social)](https://cloud-native.slack.com/archives/C04LGHDR9K7)
[![codecov](https://codecov.io/gh/knative/serving/branch/main/graph/badge.svg)](https://codecov.io/gh/knative/serving)
[![CII Best Practices](https://bestpractices.coreinfrastructure.org/projects/5913/badge)](https://bestpractices.coreinfrastructure.org/projects/5913)

Knative Serving builds on Kubernetes to support deploying and serving of
applications and functions as serverless containers. Serving is easy to get
started with and scales to support advanced scenarios.

This repository is a fork of the original Knative-serving project. It extends
the original framework with the definition and deployment of a custom scheduler controller.
For the implementation of the custom scheduler controller, see [its GitHub repository](https://github.com/Tarik-Kada/custom-scheduler-controller).
The updated framework and its custom scheduler can be easily controlled through the [ks-cs-dashboard](https://github.com/Tarik-Kada/kc-cs-dashboard).

This repo also contains som [Bash scripts](https://github.com/Tarik-Kada/knative-serving/blob/main/hack) to get users started.
KinD (Kubernetes in Docker) can be used to run a local cluster. The script uses the Kubernetes Command Line Tool (kubectl)
to deploy the necessary services. The script also deploys [the Prometheus stack](https://github.com/prometheus-community/helm-charts/tree/main/charts/kube-prometheus-stack)
to the cluster. This can be removed, but this will disable the functionality of passing custom queries
to their deployed custom scheduling algorithm. [Helm](https://helm.sh/docs/intro/install/) is required to deploy the Prometheus stack successfully.

Run [the `deploy.sh` script](https://github.com/Tarik-Kada/knative-serving/blob/main/hack/deploy.sh) found in [the hack repository](https://github.com/Tarik-Kada/knative-serving/blob/main/hack) after creating the cluster using [the create_cluster.sh script](https://github.com/Tarik-Kada/knative-serving/blob/main/hack/quickstart_cluster/create_cluster.sh) for a quick start. Deploy [the custom scheduler controller](https://github.com/Tarik-Kada/custom-scheduler-controller/) to support the use of external scheduling algorithms.


The Knative Serving project provides middleware primitives that enable:

- Rapid deployment of serverless containers
- Automatic scaling up and down to zero
- Routing and network programming
- Point-in-time snapshots of deployed code and configurations

For documentation on using Knative Serving, see the
[serving section](https://www.knative.dev/docs/serving/) of the
[Knative documentation site](https://www.knative.dev/docs).

For documentation on the Knative Serving specification, see the
[docs](https://github.com/knative/serving/tree/main/docs) folder of this
repository.

If you are interested in contributing, see [CONTRIBUTING.md](./CONTRIBUTING.md)
and [DEVELOPMENT.md](./DEVELOPMENT.md). For a list of all help wanted issues
across Knative, take a look at [CLOTRIBUTOR](https://clotributor.dev/search?project=knative&page=1).
