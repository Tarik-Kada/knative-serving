#!/bin/bash

# Delete the cluster
kind delete cluster --name knative

echo "Cluster knative has been deleted."
