LDFLAGS := -X github.com/nais/tobac/pkg/version.Revision=$(shell git rev-parse --short HEAD) -X github.com/nais/tobac/pkg/version.Version=$(shell /bin/cat ./version)

build:
	go build

release:
	go build -a -installsuffix cgo -o tobac -ldflags "-s $(LDFLAGS)"

docker:
	docker build -t navikt/tobac .
