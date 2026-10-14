FROM golang:1.27.2-bookworm@sha256:55395706e9703db746cc507abfc4eb2aea75918f8a8024e4848e2cb81004f5ad AS build
WORKDIR /src
COPY go.mod go.sum ./
COPY stages/m0/pricing-api ./stages/m0/pricing-api
ARG APP_VERSION=v1
RUN case "$APP_VERSION" in v1|v2-good|v2-broken) ;; *) exit 1 ;; esac \
    && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
       -ldflags="-s -w -X main.version=$APP_VERSION" \
       -o /pricing-api ./stages/m0/pricing-api

FROM scratch AS runtime
COPY --from=build /pricing-api /pricing-api
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/pricing-api"]
CMD ["--listen=:8080"]
