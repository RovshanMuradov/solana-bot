# ./deploy/Dockerfile
FROM golang:1.20-alpine

WORKDIR /app
COPY . .

RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /bot ./cmd/bot/main.go

FROM alpine:latest
COPY --from=0 /bot /bot
COPY configs/ /configs/

ENTRYPOINT ["/bot"]