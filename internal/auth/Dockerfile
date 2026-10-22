FROM golang:1.26 AS base

WORKDIR /app

COPY go.sum ./
COPY go.mod ./

RUN go mod download

COPY config ./config

COPY handler.go ./
COPY service.go ./
COPY repository.go ./
COPY const.go ./
COPY checks.go ./
COPY model.go ./
COPY config ./config
COPY migrations ./migrations

RUN CGO_ENABLED=0 go build -o auth ./cmd/auth
RUN CGO_ENABLED=0 go build -o migrate ./cmd/migrations

FROM alpine:latest
WORKDIR /app

COPY --from=base /app/migrations ./migrations
COPY --from=base /app/auth ./auth
COPY --from=base /app/migrate ./migrate
COPY --from=base /app/web ./web
ENTRYPOINT ["./auth"]