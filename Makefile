export GO111MODULE=on

LINT_VERSION := v1.16.0
HAS_LINT := $(shell command -v golangci-lint 2> /dev/null)

test-redis-queue:
	docker-compose up --build  --abort-on-container-exit
	docker-compose down 

lint:
ifndef HAS_LINT
	curl -sfL https://install.goreleaser.com/github.com/golangci/golangci-lint.sh | sh -s -- -b $(GOPATH)/bin $(LINT_VERSION)
endif
	golangci-lint run \
		--disable-all=true \
		--enable=errcheck \
		./...

format:
	go fmt ./...
