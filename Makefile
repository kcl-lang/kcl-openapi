# note: call scripts from /scripts
PROJECT_ROOT:=$(shell pwd)
GO_FILES:=$$(find ./ -type f -name '*.go' -not -path ".//vendor/*")
COVER_FILE			?= coverage.out
SOURCE_PATHS		?= ./pkg/...

build-local-all: build-darwin build-darwin-arm64 build-linux build-windows

build-local:
	rm -rf ${PROJECT_ROOT}/_build/bin
	mkdir -p ${PROJECT_ROOT}/_build/bin
	go build -o ${PROJECT_ROOT}/_build/bin/kcl-openapi ${PROJECT_ROOT}

build-darwin:
	rm -rf ${PROJECT_ROOT}/_build/bin/darwin
	mkdir -p ${PROJECT_ROOT}/_build/bin/darwin
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -o ${PROJECT_ROOT}/_build/bin/darwin/kcl-openapi ${PROJECT_ROOT}

build-darwin-arm64:
	rm -rf ${PROJECT_ROOT}/_build/bin/darwin-arm64
	mkdir -p ${PROJECT_ROOT}/_build/bin/darwin-arm64
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o ${PROJECT_ROOT}/_build/bin/darwin-arm64/kcl-openapi ${PROJECT_ROOT}

build-linux:
	rm -rf ${PROJECT_ROOT}/_build/bin/linux
	mkdir -p ${PROJECT_ROOT}/_build/bin/linux
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ${PROJECT_ROOT}/_build/bin/linux/kcl-openapi ${PROJECT_ROOT}

build-windows:
	rm -rf ${PROJECT_ROOT}/_build/bin/windows
	mkdir -p ${PROJECT_ROOT}/_build/bin/windows
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o ${PROJECT_ROOT}/_build/bin/windows/kcl-openapi.exe ${PROJECT_ROOT}

test:
	go test ${SOURCE_PATHS}

cover:  ## Generates coverage report
	go test -gcflags=all=-l -timeout=10m `go list $(SOURCE_PATHS)` -coverprofile $(COVER_FILE) ${TEST_FLAGS}

vet-fmt:
	go vet ./...
	go fmt ./...

clean:
	rm -rf models
	rm -rf test_data/tmp_*

check-fmt:
	test -z $$(goimports -l -w -e -local=kusionstack.io $(GO_FILES))

regenerate:
	go run scripts/regenerate.go
