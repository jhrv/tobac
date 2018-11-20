build:
	go build

local:
	go run main.go -logtostderr

docker:
	docker build -t navikt/tobac .
