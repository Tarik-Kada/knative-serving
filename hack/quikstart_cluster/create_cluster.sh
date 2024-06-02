#!/bin/bash

# Create the cluster
kind create cluster --name knative --config ./kind-config.yaml

# Configure kubectl to use the new cluster
kubectl cluster-info --context kind-knative

echo "Cluster my-cluster has been created and is ready for use."