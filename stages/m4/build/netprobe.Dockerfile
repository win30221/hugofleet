FROM golang:1.27.2-bookworm@sha256:55395706e9703db746cc507abfc4eb2aea75918f8a8024e4848e2cb81004f5ad AS build
WORKDIR /src
COPY go.mod go.sum ./
COPY stages/m4/cmd/netprobe ./stages/m4/cmd/netprobe
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o /netprobe ./stages/m4/cmd/netprobe
FROM scratch
COPY --from=build /netprobe /netprobe
USER 65532:65532
ENTRYPOINT ["/netprobe"]
