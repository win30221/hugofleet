FROM golang:1.27.2-bookworm@sha256:55395706e9703db746cc507abfc4eb2aea75918f8a8024e4848e2cb81004f5ad AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY stages/m1 ./stages/m1
COPY stages/m2 ./stages/m2
COPY stages/m3 ./stages/m3
COPY stages/m5/observability ./stages/m5/observability
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /hub ./stages/m2/cmd/hub
FROM scratch
COPY --from=build /hub /hub
COPY stages/m2/policy.json /policy.json
USER 65532:65532
ENTRYPOINT ["/hub"]
