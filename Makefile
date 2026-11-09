.PHONY: all test vet build linux-arm64 web web-check e2e sim dev restart clean

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

# The server for photon (EC2, Linux arm64): bin/parallax-linux-arm64.
linux-arm64: web
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags embed -trimpath -ldflags="-s -w" -o bin/parallax-linux-arm64 ./cmd/server

sim:
	go run ./cmd/sim $(ARGS)

# Run the server on :8080; in another terminal run `cd web && npm run dev`
# and open the URL Vite prints.
dev:
	go run ./cmd/server -dev

# Stop any running server, rebuild and start again (scripts/restart.sh).
restart:
	scripts/restart.sh $(ARGS)

clean:
	rm -rf bin web/dist
