APP=gha
VERSION?=dev
GO?=go
DIST?=dist

.PHONY: all build test lint fmt tidy clean release run

all: test build

build:
	$(GO) build -ldflags "-X github.com/zhongtait/gh-account/cmd.version=$(VERSION)" -o bin/$(APP) ./cmd/gha

run:
	$(GO) run ./cmd/gha

test:
	$(GO) test ./...

fmt:
	$(GO) fmt ./...

tidy:
	$(GO) mod tidy

lint:
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run ./... || $(GO) vet ./...

clean:
	rm -rf bin $(DIST)

release:
	./scripts/build-release.sh $(VERSION)
