.PHONY: all test vet build web web-check e2e sim dev clean

all: vet test build

test:
	go test ./...

vet:
	go vet ./...

web/node_modules: web/package.json web/package-lock.json
	cd web && npm ci
	touch web/node_modules

web: web/node_modules
	cd web && npm run build

web-check: web/node_modules
	cd web && npm run check

# Browser smoke test: builds the client and server, then plays two browsers.
e2e: web/node_modules
	cd web && npm run e2e

# One binary with the web client embedded.
build: web
	go build -tags embed -o bin/server ./cmd/server
	go build -o bin/sim ./cmd/sim

sim:
	go run ./cmd/sim $(ARGS)

# Run the server on :8080; in another terminal run `cd web && npm run dev`
# and open the URL Vite prints.
dev:
	go run ./cmd/server -dev

clean:
	rm -rf bin web/dist
