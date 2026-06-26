# Build a static server binary. templ output and the compiled Tailwind CSS are
# committed and embedded via embed.FS, so the build only needs the Go toolchain.
FROM golang:1.25-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /server ./cmd/server

# Minimal runtime image — static binary, no libc needed.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /server /server
EXPOSE 8080
ENTRYPOINT ["/server", "-addr", ":8080"]
