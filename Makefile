EXAMPLE=./example
export EXAMPLE_MODEL=${EXAMPLE}/model
SRC=$(find . -name "*.go")
BASE_PACKAGE := github.com/kberov/rowx

ifeq (, $(which golangci-lint))
$(echo "could not find golangci-lint in $(PATH), run: curl -sfL https://install.goreleaser.com/github.com/golangci/golangci-lint.sh | sh")
endif

.PHONY: fmt lint test install_deps clean update_deps

default: all

all: fmt test rowx

fmt:
	$(info ******************** checking formatting ********************)
	go list -f '{{.Dir}}' ./... | xargs -I {} goimports -local $(BASE_PACKAGE) -w {}

lint:
	$(info ******************** running lint tools ********************)
	golangci-lint run --config ./.golangci.yaml # -v

test: install_deps clean
	$(info ******************** running tests ********************)
	go test -failfast -v  ./ ./... -coverprofile=coverage.html
	# test if the produced EXAMPLE_MODEL compiles too
	go build ./...
	go tool cover -html=coverage.html

install_deps:
	$(info ******************** downloading dependencies ********************)
	go get -v ./...

clean:
	rm -rf rx/$(EXAMPLE)
	rm -rf rx/testdata/$(EXAMPLE)
	rm -rfv *.sqlite
	rm -rfv */**/*.sqlite
	rm -rfv rowx

update_deps:
	go get -u -t -v ./...
	go mod tidy

rowx: *.go rx/*.go
	go build -ldflags '-s -w'
