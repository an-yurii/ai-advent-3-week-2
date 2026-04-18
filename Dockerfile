# Single-stage build
FROM golang:1.25.4-alpine

WORKDIR /app

# Install build dependencies for CGO (required for go-sqlite3)
RUN apk add --no-cache gcc musl-dev sqlite-dev

COPY . .

RUN go mod download
RUN CGO_ENABLED=1 GOOS=linux go build -o app ./cmd/server

EXPOSE 8080

CMD ["./app"]