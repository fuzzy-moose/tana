FROM node:24-alpine@sha256:50c8e8ca1d27439048670df5883f32d57cf81cff6233222c893fd0d9884cbd81 AS web-build
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.27.1-alpine@sha256:cf6fca6641884b8433441b2b0652976f975e1d0fdd26d177eaaf8596087f3125 AS go-build
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
    && mkdir -p /out/data /out/tmp /out/downloads

FROM scratch AS runtime
USER 65532:65532
COPY --from=go-build --chown=65532:65532 /out/data /data
COPY --from=go-build --chown=65532:65532 /out/tmp /tmp
COPY --from=go-build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
VOLUME ["/data"]

FROM runtime AS collector
COPY --from=go-build --chown=65532:65532 /out/downloads /downloads
VOLUME ["/downloads"]
ENV TANA_COLLECTOR_HOST=0.0.0.0
ENV TANA_COLLECTOR_PORT=8080
ENV TANA_COLLECTOR_DATA_DIR=/data
ENV TANA_COLLECTOR_DOWNLOAD_DIR=/downloads
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
