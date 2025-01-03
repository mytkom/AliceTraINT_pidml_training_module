.PHONY: build
build:
	go build -o bin/AliceTraINT_pidml_training_module ./cmd/AliceTraINT_pidml_training_module

.PHONY: run
run:
	go run ./cmd/AliceTraINT_pidml_training_module

.PHONY: mock
mock:
	go run ./cmd/mock

.PHONY: lint
lint:
	golangci-lint run

