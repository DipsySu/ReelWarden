.PHONY: dev server web test go-test web-build

dev: server

server:
	go run ./apps/server -config config.example.yaml

web:
	npm --prefix . run dev -- --config apps/web/vite.config.ts

test: go-test web-build

go-test:
	go test ./...

web-build:
	npm install
	npm run build -- --config apps/web/vite.config.ts
