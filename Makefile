.PHONY: all build hub agent web tidy dev clean run

GO     ?= go
NPM    ?= npm
BIN    := bin
WEBDIR := web

all: build

build: hub agent web ## build everything (hub binary, agent binary, web assets)

hub: ## build the hub binary
	$(GO) build -o $(BIN)/aperture-hub ./cmd/hub

agent: ## build the agent binary (placeholder in v0.1)
	$(GO) build -o $(BIN)/aperture-agent ./cmd/agent

web: ## build the SvelteKit frontend into web/build
	cd $(WEBDIR) && $(NPM) install --no-audit --no-fund && $(NPM) run build

tidy: ## go mod tidy
	$(GO) mod tidy

run: hub ## run the hub (after `make web`) at http://localhost:8080
	$(BIN)/aperture-hub -web-dir $(WEBDIR)/build

dev: ## run hub (API only, dev CORS) on :8080 — pair with `cd web && npm run dev` on :5173
	$(GO) run -tags dev ./cmd/hub -interval 2s

clean:
	rm -rf $(BIN) $(WEBDIR)/build $(WEBDIR)/.svelte-kit
