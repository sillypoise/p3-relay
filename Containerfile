FROM docker.io/library/golang:1.25.0-alpine AS build
WORKDIR /source
COPY go.mod ./
COPY cmd ./cmd
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /relay-api ./cmd/api

FROM docker.io/library/alpine:3.23
RUN addgroup -S relay && adduser -S -G relay relay
COPY --from=build --chown=relay:relay /relay-api /usr/local/bin/relay-api
USER relay
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/relay-api"]
