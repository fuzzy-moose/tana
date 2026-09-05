# Tana

Comic management and reading locally, with a separate cloud metadata collector.

## Run

```sh
go run ./cmd/tana
go run ./cmd/collector
```

## Build

```sh
go build -o bin/ ./cmd/...
```

This produces `bin/tana` and `bin/collector`.

## Check

```sh
go fmt ./...
go vet ./...
go test ./...
```
