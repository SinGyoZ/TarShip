VERSION ?= dev
LDFLAGS := -s -w -X github.com/SinGyoZ/TarShip/internal/version.Version=$(VERSION)

.PHONY: web build test vet docker dev-hub dev-web

web: ## 构建前端到 web/dist（会被 go:embed 内嵌）
	cd web && npm ci && npm run build

build: web ## 构建单个二进制 bin/tarship
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/tarship ./cmd/tarship

test: ## 运行 Go 测试
	go test ./...

vet:
	go vet ./...

docker: ## 构建 Docker 镜像
	docker build --build-arg VERSION=$(VERSION) -t singyoku/tarship:$(VERSION) .

dev-hub: ## 本地开发：启动 Hub（数据在 ./data）
	go run ./cmd/tarship hub --addr :8080 --data ./data

dev-web: ## 本地开发：启动前端开发服务器（代理到 :8080）
	cd web && npm run dev
