FROM node:24-alpine@sha256:e67514e5d0f6c46656005e1b693b2ec9d52e80b641307de684d4a015ba7a4eaf AS web-build
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.27.0-alpine@sha256:4c9fe60190a2a3350ddc51de80d0224b8a6698d12bdfc999fee45ea9d6c46dbc AS go-build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -o /out/tana ./cmd/tana \
    && CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -o /out/collector ./cmd/collector \
    && mkdir -p /out/data /out/tmp

FROM scratch AS runtime
USER 65532:65532
COPY --from=go-build --chown=65532:65532 /out/data /data
COPY --from=go-build --chown=65532:65532 /out/tmp /tmp
COPY --from=go-build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
VOLUME ["/data"]

FROM runtime AS collector
ENV TANA_COLLECTOR_HOST=0.0.0.0
ENV TANA_COLLECTOR_PORT=8080
ENV TANA_COLLECTOR_DATA_DIR=/data
EXPOSE 8080
COPY --from=go-build /out/collector /collector
ENTRYPOINT ["/collector"]

FROM runtime AS tana
ENV TANA_HOST=0.0.0.0
ENV TANA_PORT=8080
ENV TANA_DATA_DIR=/data
ENV TANA_WEB_DIR=/web
EXPOSE 8080
COPY --from=go-build /out/tana /tana
COPY --from=web-build /src/web/dist /web
ENTRYPOINT ["/tana"]
