## Contributing

mxnet-operator uses go modules and supports both v1beta1 and v1. We suggest to use golang 1.13.x for development. 
Make sure you enable `GO111MODULE`.

### Setup development environment

```shell
mkdir -p ${GOPATH}/src/github.com/kubeflow
cd ${GOPATH}/src/github.com/kubeflow
git clone https://github.com/${GITHUB_USER}/mxnet-operator.git
```

### Download go mods

Some utility modules like `code-generator` will be used in hack scripts, 
it's better to download all dependencies for the first time.

```shell
go mod download
```

### Build the operator locally

```shell
go build -o mxnet-operator.v1beta1 github.com/kubeflow/mxnet-operator/cmd/mxnet-operator.v1beta1
go build -o mxnet-operator.v1 github.com/kubeflow/mxnet-operator/cmd/mxnet-operator.v1
```

### Build container image

```shell
# It requires you to build binary locally first.
docker build -t ${your_dockerhub_username}/mxnet-operator:v1 .
```

### Test Binaries locally

```shell
./mxnet-operator.v1beta1 --kubeconfig=$HOME/.kube/config
```

### Before code check-in

There're several steps to follow before you submit PR.

```shell
# Verify codegen in case you change api but for get to generate new packages.
./hack/verify-codegen.sh

# Lint codes
golangci-lint run --config=linter_config.yaml ./...
```

### Regenerate clients and apis

If you make changes under `/pkg/apis`, you probably need to regenerate clients and apis.

```shell
./hack/update-codegen.sh
```