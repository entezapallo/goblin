SHELL := /bin/bash

.DEFAULT_GOAL := build
BINDIR := _build
BINARIES := gash summon goblin gguard raven orb grimoire
INSTDIR := $(HOME)/.local/bin

.PHONY: build
build: ## Compila todos los binarios en _build/
	@mkdir -p $(BINDIR)
	@for b in $(BINARIES); do \
		go build -o $(BINDIR)/$$b ./cmd/$$b || exit 1; \
		printf '  %-10s ok\n' $$b; \
	done

.PHONY: test
test: ## Tests unitarios + de integración con cgroupfs
	@go test ./... -count=1

.PHONY: test-gg
test-gg: ## Prueba end-to-end de ggroups contra cgroupfs real
	@./scripts/gg_test.sh

.PHONY: vet
vet: ## go vet
	@go vet ./...

.PHONY: fmt
fmt: ## gofmt de todo el árbol
	@gofmt -w cmd pkg

.PHONY: check
check: fmt vet test ## Todo junto antes de commitear

.PHONY: install
install: build ## Instala binarios y symlinks de grimoire en ~/.local/bin
	@mkdir -p $(INSTDIR)
	@for b in $(BINARIES); do install -m 755 $(BINDIR)/$$b $(INSTDIR)/$$b; done
	@for a in ash awk cat cp cut diff echo find grep sed sort tr uniq wc ps kill pgrep pstree; do \
		ln -sf grimoire $(INSTDIR)/$$a; \
	done
	@echo "Instalado en $(INSTDIR)"

.PHONY: sizes
sizes: ## Tamaño de cada binario
	@mkdir -p $(BINDIR)
	@for b in $(BINARIES); do \
		go build -o $(BINDIR)/$$b ./cmd/$$b || exit 1; \
		printf '%-10s %8s\n' $$b "$$(stat -c%s $(BINDIR)/$$b | numfmt --to=iec 2>/dev/null || stat -c%s $(BINDIR)/$$b)"; \
	done

.PHONY: clean
clean: ## Borra artefactos
	@rm -rf $(BINDIR)
