1. minikube start --driver=podman
2. minikube image build -t ofcir.io/ofcir:latest .
3. make generate-deploy-manifests
4. kubectl apply -f ofcir-manifests/