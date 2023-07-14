build:
	go build -o load-balancer
	go mod tidy
run:
	go run load-balancer.go

clean:
	rm -f load-balancer

docker-build-server1:
	docker build -t server1 ./server1

docker-build-server2:
	docker build -t server2 ./server2

docker-build-server3:
	docker build -t server3 ./server3

docker-build:
	docker build -t load-balancer .

docker-up:
	docker-compose up --build

docker-down:
	docker-compose down

prometheus-up:
	docker-compose -f docker-compose.yaml -f prometheus.yml up

prometheus-down:
	docker-compose -f docker-compose.yaml -f prometheus.yml down

all: build docker-build

docker-build:
	find . -name "Dockerfile" -execdir docker build -t load-balancer:{} \;
