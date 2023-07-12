FROM golang:1.17-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o loadbalancer loadbalancer.go

FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/loadbalancer .

EXPOSE 8000

CMD ["./loadbalancer"]
