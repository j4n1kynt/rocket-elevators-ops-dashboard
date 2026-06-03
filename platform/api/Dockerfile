# AND-105 Task 1: Docker and PostgreSQL Setup

# Stage 1: build
FROM golang:1.25-alpine AS builder

WORKDIR /build

COPY platform/api/go.mod platform/api/go.sum ./
RUN go mod download

COPY platform/api/*.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -o api .

# Stage 2: runtime
FROM alpine:3.19

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=builder /build/api ./api

EXPOSE 8080

CMD ["./api"]
