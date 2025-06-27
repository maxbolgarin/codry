LDFLAGS += -X 'main.Branch=$(shell git rev-parse --abbrev-ref HEAD)'
LDFLAGS += -X 'main.Commit=$(shell git rev-parse HEAD)'
LDFLAGS += -X 'main.BuildDate=$(shell LANG=en_us_88591; date)'
LDFLAGS += -X 'main.Version=$(shell git describe --tags --always)'

run:
	go run -ldflags="${LDFLAGS}" cmd/main/main.go -c compose/config.yml

start: run

t:
	go test -race ./...


upd:
	go get gitlab.158-160-60-159.sslip.io/astra-monitoring-icl/go-lib
	go mod tidy

lint:
	golangci-lint run -v ./...