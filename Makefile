.PHONY: dev server web test go-test web-build

dev:
	$(MAKE) -j2 server web

server:
	go run ./apps/server -config config.example.yaml

web:
	npm run dev

test: go-test web-build

go-test:
	go test ./...

web-build:
	npm install
	npm run build
