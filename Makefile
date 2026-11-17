SHELL := /bin/bash

.DEFAULT_GOAL := build

.PHONY: build
build: ## Intenta compilar todo el arbol tal cual esta (detecta imports rotos)
	@go build ./... 2>&1 | tee /tmp/build-errors.log; \
	if [ -s /tmp/build-errors.log ]; then \
		echo ""; echo "=== BUILD FALLO, ver errores arriba ==="; exit 1; \
	fi; \
	echo "BUILD OK"

.PHONY: vet
vet: ## Igual que build pero con go vet (a veces pesca cosas distintas)
	@go vet ./...

.PHONY: sizes
sizes: ## Compila cada cmd/<applet> restante y muestra el tamaño de cada binario
	@mkdir -p _build/sizes
	@total=0; \
	for d in cmd/*/; do \
		name=$$(basename "$$d"); \
		if go build -o "_build/sizes/$$name" "./$$d" 2>/tmp/sizes-err-$$name.log; then \
			size=$$(stat -f%z "_build/sizes/$$name" 2>/dev/null || stat -c%s "_build/sizes/$$name"); \
			total=$$((total + size)); \
			printf '%-20s %10d bytes\n' "$$name" "$$size"; \
		else \
			printf '%-20s %10s (build failed, ver /tmp/sizes-err-%s.log)\n' "$$name" "FAIL" "$$name"; \
		fi; \
	done; \
	echo "----------------------------------------"; \
	printf 'TOTAL%16s %10d bytes (%s MB)\n' "" "$$total" "$$(echo "scale=2; $$total/1048576" | bc)"

.PHONY: clean
clean:
	@rm -f /tmp/build-errors.log /tmp/sizes-err-*.log
	@rm -rf _build/sizes
