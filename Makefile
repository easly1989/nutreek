# Makefile for Nutreek
#
# Platform variables (Windows / Unix)
# NOTE: pinned to Tailwind v3.4.17 because v4 changed the CLI/config
# (no tailwind.config.js in JS, @import instead of @tailwind, etc.).
# The upgrade to v4 gets its own task.
TAILWIND_VERSION := v3.4.17

ifeq ($(OS),Windows_NT)
	EXE := .exe
	TAILWIND_URL := https://github.com/tailwindlabs/tailwindcss/releases/download/$(TAILWIND_VERSION)/tailwindcss-windows-x64.exe
	MKDIR := powershell -NoProfile -Command New-Item -ItemType Directory -Force -Path
	DOWNLOAD = powershell -NoProfile -Command "Invoke-WebRequest -Uri '$(1)' -OutFile '$(2)'"
else
	EXE :=
	UNAME_S := $(shell uname -s)
	UNAME_M := $(shell uname -m)
	ifeq ($(UNAME_S),Linux)
		TAILWIND_OS := linux
	endif
	ifeq ($(UNAME_S),Darwin)
		TAILWIND_OS := macos
	endif
	ifeq ($(UNAME_M),x86_64)
		TAILWIND_ARCH := x64
	endif
	ifeq ($(UNAME_M),arm64)
		TAILWIND_ARCH := arm64
	endif
	ifeq ($(UNAME_M),aarch64)
		TAILWIND_ARCH := arm64
	endif
	TAILWIND_URL := https://github.com/tailwindlabs/tailwindcss/releases/download/$(TAILWIND_VERSION)/tailwindcss-$(TAILWIND_OS)-$(TAILWIND_ARCH)
	MKDIR := mkdir -p
	DOWNLOAD = curl -sSL -o $(2) $(1)
endif

BIN_DIR := bin
TAILWIND_BIN := $(BIN_DIR)/tailwindcss$(EXE)

TAILWIND_INPUT := web/tailwind/input.css
TAILWIND_CONFIG := web/tailwind/tailwind.config.js
TAILWIND_OUTPUT := internal/web/static/css/tailwind.css

.PHONY: help
help:
	@echo "Nutreek - available targets:"
	@echo "  setup          download local tools (Tailwind CLI)"
	@echo "  generate       generate the CSS (alias of tailwind)"
	@echo "  tailwind       build the Tailwind CSS once"
	@echo "  tailwind-watch build the CSS in watch mode"
	@echo "  dev            run the app in dev (go run)"
	@echo "  build          build the binary into bin/nutreek$(EXE)"
	@echo "  test           run go test ./..."
	@echo "  docker         build the Docker image"
	@echo "  docker-up      docker compose up --build"
	@echo "  clean          remove bin/ and the generated CSS"

.PHONY: setup
setup: $(TAILWIND_BIN)
	@echo "Setup done."

$(TAILWIND_BIN):
	@$(MKDIR) $(BIN_DIR)
	@echo "Downloading the tailwindcss CLI from $(TAILWIND_URL)..."
	@$(call DOWNLOAD,$(TAILWIND_URL),$(TAILWIND_BIN))
ifeq ($(OS),Windows_NT)
else
	@chmod +x $(TAILWIND_BIN)
endif

.PHONY: tailwind
tailwind: $(TAILWIND_BIN)
	@$(MKDIR) internal/web/static/css
	$(TAILWIND_BIN) -c $(TAILWIND_CONFIG) -i $(TAILWIND_INPUT) -o $(TAILWIND_OUTPUT) --minify

.PHONY: tailwind-watch
tailwind-watch: $(TAILWIND_BIN)
	@$(MKDIR) internal/web/static/css
	$(TAILWIND_BIN) -c $(TAILWIND_CONFIG) -i $(TAILWIND_INPUT) -o $(TAILWIND_OUTPUT) --watch

.PHONY: generate
generate: tailwind

.PHONY: dev
dev: tailwind
	NUTREEK_ENV=dev go run ./cmd/nutreek

.PHONY: build
build: tailwind
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(BIN_DIR)/nutreek$(EXE) ./cmd/nutreek

.PHONY: test
test:
	go test ./...

.PHONY: docker
docker:
	docker build -t nutreek:latest .

.PHONY: docker-up
docker-up:
	docker compose up --build

.PHONY: clean
clean:
	@rm -rf $(BIN_DIR) $(TAILWIND_OUTPUT) coverage.out coverage.html
