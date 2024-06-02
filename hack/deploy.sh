# Deploy cert-manager
echo "Deploying cert-manager..."
echo "---------------------------------------------------------------"
kubectl apply -f ./third_party/cert-manager-latest/cert-manager.yaml
kubectl wait --for=condition=Established --all crd
kubectl wait --for=condition=Available -n cert-manager --all deployments
echo "cert-manager has been deployed successfully."
echo "---------------------------------------------------------------"

# Set KO_DOCKER_REPO to the kind cluster
# export KO_DOCKER_REPO="kind.local"
export KO_DOCKER_REPO='docker.io/tarikkada'
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

# Forwarding ports for service invocation

# Wait for pods to be ready
echo "Waiting for pods to be ready..."
kubectl wait --for=condition=Ready -n kourier-system --all pods
echo "Forwarding ports (:8080) for service invocation..."
kubectl port-forward -n kourier-system svc/kourier 8080:80
