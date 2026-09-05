FROM docker.io/library/node:24.13.0-alpine AS web-build
WORKDIR /source/web
RUN npm install --global pnpm@10.33.2
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY web ./
RUN pnpm build

FROM docker.io/library/golang:1.25.10-alpine AS go-build
WORKDIR /source
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -mod=readonly -trimpath -o /out/relay-api ./cmd/api \
    && CGO_ENABLED=0 go build -mod=readonly -trimpath -o /out/relay-worker ./cmd/worker \
    && CGO_ENABLED=0 go build -mod=readonly -trimpath -o /out/relay-migrate ./cmd/migrate

FROM docker.io/library/alpine:3.23 AS runtime
RUN apk add --no-cache ca-certificates \
    && addgroup -S -g 10001 relay \
    && adduser -S -D -H -u 10001 -G relay relay
WORKDIR /app
COPY --from=go-build /out/ /usr/local/bin/
COPY --from=web-build /source/web/dist/ /app/web/
COPY migrations/ /app/migrations/
ENV RELAY_WEB_DIRECTORY=/app/web
USER 10001:10001
EXPOSE 8080
# ECS can override the command for the worker or the one-off migration task.
CMD ["/usr/local/bin/relay-api"]
