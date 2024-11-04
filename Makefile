GO_FILES:=$$(find ./ -type f -name '*.go' -not -path ".//vendor/*")
COVER_FILE    ?= coverage.out
SOURCE_PATHS  ?= ./pkg/...

test:
	go test ./...

cover:  ## Generates coverage report
	go test -gcflags=all=-l -timeout=10m `go list $(SOURCE_PATHS)` -coverprofile $(COVER_FILE) ${TEST_FLAGS}

clean:
	rm -rf models
	rm -rf test_data/tmp_*

check-fmt:
	test -z $$(goimports -l -w -e -local=kcl-lang.io $(GO_FILES))
	gofmt -l -w .

regenerate: ## regenerate all the golden files
	go run scripts/regenerate.go

build-local-linux:
	# Delete old artifacts
	-rm -rf ./_build/linux
	mkdir -p ./_build/linux/

	# Build kcl-openapi
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
		go build -o ./_build/linux/kcl-openapi \
		-ldflags="-s -w" .

build:
	-rm -rf ./_build/local/
	mkdir -p ./_build/local/
	# Build kcl-openapi
	CGO_ENABLED=0 go build -o ./_build/local/kcl-openapi -ldflags="-s -w" .
