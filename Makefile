.PHONY: build

build:
	go build -C cmd/mqtunnel -o ../../mqtunnel
run_mosquitto:
	docker run -it -d -p 1883:1883 eclipse-mosquitto:2.0.15 mosquitto -c /mosquitto-no-auth.conf
