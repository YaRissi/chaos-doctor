ns := "clinic"
app := "web"

default:
    @just --list

# create the kind cluster and deploy the demo app
up:
    kind create cluster --config kind.yaml --wait 120s
    helm install {{app}} chart -n {{ns}} --create-namespace --wait --timeout 3m

down:
    kind delete cluster --name clinic

# reinstall the demo app from scratch
reset:
    -helm uninstall {{app}} -n {{ns}} --wait
    -kubectl delete namespace {{ns}} --wait=true
    helm install {{app}} chart -n {{ns}} --create-namespace --wait --timeout 3m

doctor *args:
    go run ./cmd/doctor {{args}}

lint:
    go vet ./...
    test -z "$(gofmt -l .)"
