# Single-stage build
FROM golang:1.25.4-alpine

WORKDIR /app

COPY . .

RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o app ./cmd/server

EXPOSE 8080

CMD ["./app"]