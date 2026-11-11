FROM golang:1.26 AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY gen ./gen
COPY services/auth ./services/auth
RUN CGO_ENABLED=0 go build -o /out/auth ./services/auth/cmd/auth
RUN CGO_ENABLED=0 go build -o /out/migrate ./services/auth/cmd/migrations

FROM alpine:latest
WORKDIR /app
COPY --from=build /out/auth /out/migrate ./
ENTRYPOINT ["./auth"]
