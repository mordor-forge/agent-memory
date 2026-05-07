FROM golang:1.25.9-alpine AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN mkdir -p /out \
    && CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/memoryd ./cmd/memoryd \
    && CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/agent-memory-mcp ./cmd/agent-memory-mcp

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
COPY --from=build /out/memoryd /usr/local/bin/memoryd
COPY --from=build /out/agent-memory-mcp /usr/local/bin/agent-memory-mcp
EXPOSE 8080
ENTRYPOINT ["memoryd"]
CMD ["serve"]
