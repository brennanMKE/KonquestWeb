.PHONY: all test vet build sim clean

all: vet test build

test:
	go test ./...

vet:
	go vet ./...

build:
	go build -o bin/ ./cmd/...

sim:
	go run ./cmd/sim $(ARGS)

clean:
	rm -rf bin
