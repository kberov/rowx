BIN="./bin"
EXAMPLE=./example
export EXAMPLE_MODEL=${EXAMPLE}/generated/model
SRC=$(find . -name "*.go")
BASE_PACKAGE := github.com/kberov/rowx

ifeq (, $(which golangci-lint))
$(echo "could not find golangci-lint in $(PATH), run: curl -sfL https://install.goreleaser.com/github.com/golangci/golangci-lint.sh | sh")
endif

.PHONY: fmt lint test install_deps clean update_deps

default: all

all: fmt test

fmt:
	$(info ******************** checking formatting ********************)
	go list -f '{{.Dir}}' ./... | xargs -I {} goimports -local $(BASE_PACKAGE) -w {}

lint:
	$(info ******************** running lint tools ********************)
	golangci-lint run # -v

test: install_deps
	$(info ******************** running tests ********************)
	go test -failfast -v  ./... -coverprofile=coverage.html	
	# go tool cover -html=coverage.html
	# TODO: re-generate example model, build it with a build tag
	# `go:build example_model` and then run the generated test package.
	mkdir -p $(EXAMPLE_MODEL)
install_deps:
	$(info ******************** downloading dependencies ********************)
	go get -v ./...

clean:
	rm -rf $(BIN)
	rm -rf $(EXAMPLE)

update_deps:
	go get -u -t -v ./...
	go mod tidy
