build:
	go mod tidy
	go build -o load-balancer

run:
	go run loadbalancer.go

clean:
	rm -f load-balancer

docker-up:
	docker-compose up --build

docker-down:
	docker-compose down



