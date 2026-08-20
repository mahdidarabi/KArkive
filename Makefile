# Image of the operator (override in CI).
IMG ?= ghcr.io/mahdidarabi/karkive:dev

# Tool versions
CONTROLLER_TOOLS_VERSION ?= v0.17.3

GOBIN ?= $(shell go env GOPATH)/bin
CONTROLLER_GEN ?= $(GOBIN)/controller-gen

.PHONY: all
all: generate manifests fmt vet test build

.PHONY: build
build:
	go build -o bin/karkive ./cmd

.PHONY: run
run: generate manifests
	go run ./cmd --leader-elect=false

.PHONY: test
test: generate
	go test ./...

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: vet
vet:
	go vet ./...

.PHONY: generate
generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./api/..."

.PHONY: manifests
manifests: controller-gen generate
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." \
		output:crd:artifacts:config=config/crd/bases \
		output:rbac:artifacts:config=config/rbac \
		output:webhook:artifacts:config=config/webhook
	mkdir -p charts/karkive/crds
	cp config/crd/bases/*.yaml charts/karkive/crds/

.PHONY: docker-build
docker-build:
	docker build -t $(IMG) .

.PHONY: install
install: manifests
	kubectl apply -f config/crd/bases

.PHONY: uninstall
uninstall:
	kubectl delete -f config/crd/bases --ignore-not-found

.PHONY: controller-gen
controller-gen:
	@test -x "$(CONTROLLER_GEN)" || GOBIN=$(GOBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)
