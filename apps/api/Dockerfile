FROM golang:1.25-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build a static Linux binary for container runtime.
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM alpine:3.22

RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -S app && adduser -S app -G app

WORKDIR /app
COPY --from=builder /out/server /app/server

ENV PORT=8080
EXPOSE 8080

USER app
CMD ["/app/server"]
