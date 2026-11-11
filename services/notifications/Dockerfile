FROM golang:1.26 AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY services/notifications ./services/notifications
RUN CGO_ENABLED=0 go build -o /out/notifications ./services/notifications/cmd/notifications
RUN CGO_ENABLED=0 go build -o /out/migrate ./services/notifications/cmd/migrations

FROM alpine:latest
WORKDIR /app
COPY --from=build /out/notifications /out/migrate ./
ENTRYPOINT ["./notifications"]
