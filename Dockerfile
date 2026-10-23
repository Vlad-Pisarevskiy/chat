FROM golang:1.26 AS base

WORKDIR /app

COPY go.sum ./
COPY go.mod ./

RUN go mod download

COPY config ./config

COPY web ./web
COPY gen ./gen
COPY internal ./internal
COPY ./migrations ./migrations
COPY cmd/chat/main.go ./cmd/chat/
COPY cmd/migrations/chat ./cmd/migrations/chat

RUN CGO_ENABLED=0 go build -o chat ./cmd/chat
RUN CGO_ENABLED=0 go build -o migrate ./cmd/migrations/chat

FROM alpine:latest
WORKDIR /app

COPY --from=base /app/migrations ./migrations
COPY --from=base /app/chat ./chat
COPY --from=base /app/migrate ./migrate
COPY --from=base /app/web ./web
ENTRYPOINT ["./chat"]