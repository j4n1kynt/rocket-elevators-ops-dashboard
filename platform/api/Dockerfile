# AND-105 Task 1: Docker and PostgreSQL Setup

# Stage 1: build
FROM golang:1.22-alpine AS builder

WORKDIR /build

COPY platform/api/go.mod ./
RUN go mod download

COPY platform/api/*.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -o api .

# Stage 2: runtime
FROM alpine:3.19

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY --from=builder /build/api ./api
COPY platform/elevator_fleet.csv ./platform/elevator_fleet.csv
COPY data/ ./data/

EXPOSE 8080

CMD ["./api"]
