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

## Frontend

The React TypeScript app lives in `web/`, scaffolded with Vite's official
`react-ts` template. Use a supported Node.js release compatible with Vite
(Node.js 24 LTS recommended) and npm.

```sh
cd web
npm ci
npm run dev
```

Run frontend checks and create the production build:

```sh
npm run lint
npm run build
```

The build checks TypeScript and writes static assets to `web/dist/`.
`npm run preview` previews that build locally. The Go commands are still
placeholders; frontend serving and API integration come later.
