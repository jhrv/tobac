DATE=$(shell date "+%Y-%m-%d")
LAST_COMMIT=$(shell git --no-pager log -1 --pretty=%h)
VERSION="$(DATE)-$(LAST_COMMIT)"
LDFLAGS := -X github.com/nais/tobac/pkg/version.Revision=$(shell git rev-parse --short HEAD) -X github.com/nais/tobac/pkg/version.Version=$(VERSION)

build:
	go build

test:
	go test ./... -count=1

integration_test:
	go test ./pkg/azure/azure_test.go -tags=integration -v -count=1

release:
	go build -a -installsuffix cgo -o tobac -ldflags "-s $(LDFLAGS)"

docker:
	docker build -t navikt/tobac .
