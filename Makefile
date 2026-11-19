# get-cooked development tasks. Requires Go, Node and pnpm.
BIN := bin/get-cooked
VERSION ?= dev
LDFLAGS := -s -w -X github.com/RashKash103/get-cooked/internal/cli.Version=$(VERSION)

.PHONY: all deps web build test test-go test-web e2e lint fmt golden serve example screenshots icons clean

all: lint test build

deps:
	pnpm install --frozen-lockfile

web: deps
	pnpm --filter web build

build: web
	go build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/get-cooked

test: test-go test-web

test-go: web
	go test -race ./...

test-web:
	pnpm --filter web typecheck
	pnpm --filter web test

e2e: build
	cd e2e && npx playwright install --with-deps chromium && npx playwright test

lint:
	golangci-lint run ./...
	pnpm biome check .

fmt:
	gofmt -w cmd internal docs schema
	pnpm biome check --write .

# Regenerate golden files after intentional runtime or markup changes.
golden:
	go test ./internal/expr ./internal/runtime ./internal/site -update

# Re-render the built-in PWA icon PNGs from web/public/icon.svg.
icons:
	cd e2e && node icons.mjs

# Regenerate the README screenshots (light and dark).
screenshots: build
	cd e2e && node screenshots.mjs

serve: build
	$(BIN) serve -s examples/site

example: build
	$(BIN) build -s examples/site -o examples/site/public

clean:
	rm -rf bin dist examples/site/public internal/assets/dist/*.js internal/assets/dist/*.css
