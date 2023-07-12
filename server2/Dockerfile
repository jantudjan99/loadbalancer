FROM golang:latest
WORKDIR /app
COPY . .
RUN go build -o server2 server2.go
CMD ["./server2"]