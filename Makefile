# note: call scripts from /scripts
PROJECT_ROOT:=$(shell pwd)
GO_FILES:=$$(find ./ -type f -name '*.go' -not -path ".//vendor/*")

build-local-all: build-darwin build-darwin-arm64 build-linux build-windows

build-local:
	chmod +x ${PROJECT_ROOT}/scripts/build.sh
	PROJECT_ROOT=${PROJECT_ROOT} ${PROJECT_ROOT}/scripts/build.sh

build-darwin:
	rm -rf ${PROJECT_ROOT}/_build/bin/darwin
	mkdir -p ${PROJECT_ROOT}/_build/bin/darwin
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -mod=vendor -o ${PROJECT_ROOT}/_build/bin/darwin/kclopenapi ${PROJECT_ROOT}

build-darwin-arm64:
	rm -rf ${PROJECT_ROOT}/_build/bin/darwin-arm64
	mkdir -p ${PROJECT_ROOT}/_build/bin/darwin-arm64
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -mod=vendor -o ${PROJECT_ROOT}/_build/bin/darwin-arm64/kclopenapi ${PROJECT_ROOT}

build-linux:
	rm -rf ${PROJECT_ROOT}/_build/bin/linux
	mkdir -p ${PROJECT_ROOT}/_build/bin/linux
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -mod=vendor -o ${PROJECT_ROOT}/_build/bin/linux/kclopenapi ${PROJECT_ROOT}

build-windows:
	rm -rf ${PROJECT_ROOT}/_build/bin/windows
	mkdir -p ${PROJECT_ROOT}/_build/bin/windows
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -mod=vendor -o ${PROJECT_ROOT}/_build/bin/windows/kclopenapi ${PROJECT_ROOT}

test: unit-test integration-test

unit-test:
	chmod +x ${PROJECT_ROOT}/scripts/unit_test.sh
	PROJECT_ROOT=${PROJECT_ROOT} ${PROJECT_ROOT}/scripts/unit_test.sh

integration-test:
	@echo "1. build binary ..."
	@make build-local
	@echo "2. run integration test ..."
	chmod +x ${PROJECT_ROOT}/scripts/integration_test.sh
	PROJECT_ROOT=${PROJECT_ROOT} ${PROJECT_ROOT}/scripts/integration_test.sh

test-local:
	go vet ./...
	go fmt ./...
	@echo "test"
	@make unit-test
	@make integration-test

clean:
	rm -rf models
	rm -rf test_data/tmp_*

check-fmt:
	test -z $$(goimports -l -w -e -local=kusionstack.io/kcl-openapi $(GO_FILES))

regenerate:
	go run scripts/regenerate.go
