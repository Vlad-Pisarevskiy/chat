FROM golang:1.26 AS base

WORKDIR /app

COPY go.sum ./
COPY go.mod ./

RUN go mod download

COPY gen ./gen

COPY internal/auth ./internal/auth

COPY cmd/auth/ ./cmd/auth/
COPY cmd/migrations/auth  ./cmd/migrations/auth/

RUN CGO_ENABLED=0 go build -o auth ./cmd/auth
RUN CGO_ENABLED=0 go build -o migrate ./cmd/migrations/auth

FROM alpine:latest
WORKDIR /app

COPY web/login.html web/register.html ./web/
COPY --from=base /app/internal/auth/migrations ./migrations
COPY --from=base /app/auth ./auth
COPY --from=base /app/migrate ./migrate
ENTRYPOINT ["./auth"]