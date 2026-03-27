FROM golang:1.26-alpine AS builder

WORKDIR /workspace
COPY go.mod go.sum ./
RUN go mod download

COPY api/ api/
COPY cmd/ cmd/
COPY internal/ internal/

ARG TARGETOS
ARG TARGETARCH

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags="-s -w" -trimpath -o watcher cmd/main.go

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/watcher .
USER 65532:65532

ENTRYPOINT ["/watcher"] 