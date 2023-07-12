FROM golang:latest
WORKDIR /app
COPY . .
RUN go build -o server1 server1.go
CMD ["./server1"]