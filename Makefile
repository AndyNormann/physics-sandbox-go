TAILWIND_VERSION ?= v4.3.1
TAILWIND ?= ./bin/tailwindcss
TEMPL ?= $(shell go env GOPATH)/bin/templ

# Map uname output to Tailwind release asset suffix (e.g. linux-x64, macos-arm64).
TW_OS := $(shell uname -s | tr '[:upper:]' '[:lower:]' | sed -e 's/darwin/macos/' -e 's/mingw.*/windows/' -e 's/msys.*/windows/')
TW_ARCH := $(shell uname -m | sed -e 's/x86_64/x64/' -e 's/aarch64/arm64/')
TW_ASSET := tailwindcss-$(TW_OS)-$(TW_ARCH)

.PHONY: all generate css build dev loadtest tidy clean tailwind

all: generate css build

generate:
	$(TEMPL) generate

# Bootstrap the standalone Tailwind CLI into ./bin if it's not already present.
# Override with TAILWIND=tailwindcss if you have it installed on PATH.
tailwind:
	@command -v $(TAILWIND) >/dev/null 2>&1 || ( \
		echo "downloading $(TW_ASSET) ($(TAILWIND_VERSION))..." && \
		mkdir -p $(dir $(TAILWIND)) && \
		curl -sSL -o $(TAILWIND) \
			https://github.com/tailwindlabs/tailwindcss/releases/download/$(TAILWIND_VERSION)/$(TW_ASSET) && \
		chmod +x $(TAILWIND) )

css: tailwind
	$(TAILWIND) -i tailwind.input.css -o assets/static/css/app.css --minify

build: generate css
	go build -o bin/server ./cmd/server

dev: generate css
	go run ./cmd/server

loadtest:
	go run ./cmd/loadtest -clients 50 -duration 30s

tidy:
	go mod tidy

clean:
	rm -rf bin
