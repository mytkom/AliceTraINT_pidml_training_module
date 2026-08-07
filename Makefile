.PHONY: build run mock lint subsample

build:
	go build -o bin/AliceTraINT_pidml_training_module ./cmd/AliceTraINT_pidml_training_module

run:
	go run ./cmd/AliceTraINT_pidml_training_module

mock:
	go run ./cmd/mock

lint:
	golangci-lint run

# Requires ROOT (e.g. alienv setenv O2Physics/latest -c make subsample)
subsample:
	g++ -std=c++17 -O3 -march=native subsample.cxx -o scripts/subsample $$(root-config --cflags --libs)
