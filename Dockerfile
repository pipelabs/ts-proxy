FROM golang:1.26.2-alpine AS build

WORKDIR /src

RUN apk add --no-cache ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
ARG BUILD_TIME=unknown
ARG GIT_COMMIT=unknown

RUN MODULE_NAME="$(awk '/^module / {print $2}' go.mod)" && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
      -ldflags "-s -w -X 'main.Version=${VERSION}' -X 'main.BuildTime=${BUILD_TIME}' -X 'main.GitCommit=${GIT_COMMIT}' -X 'main.ModuleName=${MODULE_NAME}'" \
      -o /out/ts-proxy . && \
    mkdir -p /out/rootfs/var/lib/ts-proxy/tsnet-state && \
    chown -R 65532:65532 /out/rootfs/var/lib/ts-proxy

FROM scratch

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build --chown=65532:65532 /out/rootfs/ /
COPY --from=build /out/ts-proxy /usr/local/bin/ts-proxy

USER 65532:65532
WORKDIR /var/lib/ts-proxy

ENV TS_PROXY_STATE_DIR=/var/lib/ts-proxy/tsnet-state

VOLUME ["/var/lib/ts-proxy/tsnet-state"]

ENTRYPOINT ["/usr/local/bin/ts-proxy"]
