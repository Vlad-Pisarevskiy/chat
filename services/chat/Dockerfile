FROM golang:1.26 AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY gen ./gen
COPY services/chat ./services/chat
RUN CGO_ENABLED=0 go build -o /out/chat ./services/chat/cmd/chat
RUN CGO_ENABLED=0 go build -o /out/migrate ./services/chat/cmd/migrations

FROM alpine:latest
WORKDIR /app
COPY --from=build /out/chat /out/migrate ./
ENTRYPOINT ["./chat"]
