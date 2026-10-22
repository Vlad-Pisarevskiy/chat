FROM golang:1.26 AS base

WORKDIR /app

COPY go.sum ./
COPY go.mod ./

RUN go mod download

COPY config ./config

COPY gen ./gen

COPY internal/auth/handler.go ./
COPY internal/auth/service.go ./
COPY internal/auth/repository.go ./
COPY internal/auth/const.go ./
COPY internal/auth/checks.go ./
COPY internal/auth/model.go ./
COPY internal/auth/config ./config
COPY internal/auth/migrations ./

COPY cmd/auth/ ./cmd/auth/
COPY cmd/migrations/auth ./cmd/migrations/auth

RUN CGO_ENABLED=0 go build -o auth ./cmd/auth
RUN CGO_ENABLED=0 go build -o migrate ./cmd/migrations/auth

FROM alpine:latest
WORKDIR /app

COPY --from=base /app/migrations ./migrations
COPY --from=base /app/auth ./auth
COPY --from=base /app/migrate ./migrate
COPY --from=base /app/web ./web
ENTRYPOINT ["./auth"]