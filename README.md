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

## Quick Start
First make sure your cluster is deployed and your kubectl context is set to this cluster. If you wish to deploy a local cluster you could use:
```bash
minikube start
```

After your cluster is up and running, run the following command to deploy the Custom Scheduler Component and all its custom resource definitions:
```bash
kubectl apply -f https://github.com/Tarik-Kada/custom-scheduler-controller/releases/download/v1.0.0/install.yaml
```

Then run the following commands to deploy the extended Knative Serving platform to your cluster:
```bash
kubectl apply -f https://github.com/Tarik-Kada/knative-serving/releases/download/v1.0.0/serving-crds.yaml
kubectl apply -f https://github.com/Tarik-Kada/knative-serving/releases/download/v1.0.0/serving-core.yaml
```

If you whish to pass custom metrics to your custom scheduling algorithm, you also have to deploy Prometheus. This can be done by running the following commands:
```bash
elm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update
helm install prometheus prometheus-community/kube-prometheus-stack -n default -f monitoring_values.yaml
kubectl apply -f https://raw.githubusercontent.com/knative-extensions/monitoring/main/servicemonitor.yaml
```

Make sure you have a local .yaml file named `monitoring_values.yaml` with the following contents:
```yaml
kube-state-metrics:
  metricLabelsAllowlist:
    - pods=[*]
    - deployments=[app.kubernetes.io/name,app.kubernetes.io/component,app.kubernetes.io/instance]
prometheus:
  prometheusSpec:
    serviceMonitorSelectorNilUsesHelmValues: false
    podMonitorSelectorNilUsesHelmValues: false

grafana:
  sidecar:
    dashboards:
      enabled: true
      searchNamespace: ALL
```

After this, you have successfully setup your deployment of the FlexSched framework. You can manage and configure the resources by running the [ks-cs-dashboard](https://github.com/Tarik-Kada/kc-cs-dashboard).

If you are interested in contributing, see [CONTRIBUTING.md](./CONTRIBUTING.md)
and [DEVELOPMENT.md](./DEVELOPMENT.md). For a list of all help wanted issues
across Knative, take a look at [CLOTRIBUTOR](https://clotributor.dev/search?project=knative&page=1).
