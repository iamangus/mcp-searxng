FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /mcp-searxng .

FROM alpine:3.21

RUN apk update && apk upgrade && apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=builder /mcp-searxng /app/mcp-searxng

ENV MCP_HTTP_PORT=3000

EXPOSE 3000

ENTRYPOINT ["/app/mcp-searxng"]
