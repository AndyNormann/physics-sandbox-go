TAILWIND ?= tailwindcss
TEMPL ?= $(shell go env GOPATH)/bin/templ

.PHONY: all generate css build dev loadtest tidy clean

all: generate css build

generate:
	$(TEMPL) generate

css:
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
