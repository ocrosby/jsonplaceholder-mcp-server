FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X github.com/ocrosby/jsonplaceholder-mcp-server/internal/version.Version=${VERSION}" \
    -o /out/jsonplaceholder-mcp-server ./cmd/jsonplaceholder-mcp-server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/jsonplaceholder-mcp-server /jsonplaceholder-mcp-server
ENV MCP_TRANSPORT=streamable-http PORT=8000
EXPOSE 8000
USER nonroot
ENTRYPOINT ["/jsonplaceholder-mcp-server"]
