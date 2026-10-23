.PHONY: run client build test fmt vet clean

## run: start the server on :9000
run:
	go run ./cmd/server

## client: start the interactive client
client:
	go run ./cmd/client

## build: compile server and client into ./bin
build:
	go build -o bin/udpserver ./cmd/server
	go build -o bin/udpclient ./cmd/client

## test: run all tests with the race detector
test:
	go test -race ./...

## fmt: format all Go code
fmt:
	gofmt -w .

## vet: run static analysis
vet:
	go vet ./...

## clean: remove build output
clean:
	rm -rf bin
