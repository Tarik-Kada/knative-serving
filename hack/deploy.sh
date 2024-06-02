# Deploy cert-manager
echo "Deploying cert-manager..."
echo "---------------------------------------------------------------"
kubectl apply -f ./third_party/cert-manager-latest/cert-manager.yaml
kubectl wait --for=condition=Established --all crd
kubectl wait --for=condition=Available -n cert-manager --all deployments
echo "cert-manager has been deployed successfully."
echo "---------------------------------------------------------------"

# Set KO_DOCKER_REPO to the kind cluster
DOCKERHUB_USERNAME="tarikkada"
export KO_DOCKER_REPO='docker.io/'$DOCKERHUB_USERNAME
echo "KO_DOCKER_REPO has been set to docker.io/tarikkada"

# Build and deploy knative serving
echo "Building and deploying knative serving..."
echo "---------------------------------------------------------------"
ko apply --selector knative.dev/crd-install=true -Rf config/core/
kubectl wait --for=condition=Established --all crd

ko apply -Rf config/core/

echo "setup sslip.io"
kubectl patch configmap/config-domain \
  --namespace knative-serving \
  --type merge \
  --patch '{"data":{"127.0.0.1.sslip.io":""}}'

# ko delete -f config/post-install/default-domain.yaml --ignore-not-found
# ko apply -f config/post-install/default-domain.yaml

echo "---------------------------------------------------------------"

# Install Kourier as the ingress controller
echo "Installing Kourier as the ingress controller..."

kubectl apply -f ./third_party/kourier-latest/kourier.yaml

kubectl patch configmap/config-network \
  -n knative-serving \
  --type merge \
  -p '{"data":{"ingress.class":"kourier.ingress.networking.knative.dev"}}'

kubectl wait --for=condition=Available -n kourier-system deployment/kourier

echo "Kourier has been installed as the ingress controller."

kubectl wait --for=condition=Ready -n knative-serving

# Deploy the Prometheus stack with Helm
echo "Installing Prometheus stack..."

helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update
helm install prometheus prometheus-community/kube-prometheus-stack -n default -f monitoring_values.yaml
echo "Prometheus stack has been installed."
echo "---------------------------------------------------------------"

# Apply the serviceMonitors/PodMonitors to collect metrics from knative
echo "Applying serviceMonitors/PodMonitors to collect metrics from knative..."
kubectl apply -f https://raw.githubusercontent.com/knative-extensions/monitoring/main/servicemonitor.yaml

echo "Loading Grafana dashboards..."
kubectl apply -f https://raw.githubusercontent.com/knative-extensions/monitoring/main/grafana/dashboards.yaml
