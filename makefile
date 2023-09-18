.PHONY: build
default: build

build:
	@echo "Building..."
	go build -o analyzet ./cmd/...
