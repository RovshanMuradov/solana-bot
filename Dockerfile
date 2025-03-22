FROM golang:1.23.2-alpine

WORKDIR /app
COPY . .

RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /bot ./cmd/bot/main.go

FROM alpine:latest
COPY --from=0 /configs/bot /bot
COPY configs/ /configs/

ENTRYPOINT ["/bot"]