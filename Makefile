# TarShip 构建入口：make build 前需先 make web（go:embed 依赖 web/dist 存在）
VERSION ?= $(shell git describe --tags --always 2>/dev/null || echo 0.1.0-dev)
LDFLAGS := -s -w -X github.com/SinGyoZ/TarShip/internal/version.Version=$(VERSION)

.PHONY: all web build test vet run clean docker help

all: web build ## 构建前端与二进制

web: ## 构建面板（产物 web/dist，go:embed 依赖）
	cd web && npm install --no-audit --no-fund && npm run build

build: ## 编译静态二进制到 bin/tarship
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/tarship ./cmd/tarship

test: vet ## 静态检查 + 单元测试
	go test ./...

vet: ## 仅静态检查
	go vet ./...

run: ## 本地启动 Hub（开发用，数据目录 .data）
	mkdir -p .data && go run ./cmd/tarship hub --addr :8080 --data .data

clean: ## 清理编译产物
	rm -rf bin

docker: ## 本地构建镜像（tag 可用 VERSION=v1.0.0 make docker 覆盖）
	docker build -t singyoku/tarship:dev --build-arg VERSION=$(VERSION) .

help: ## 显示帮助
	@grep -E '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*## "}{printf "  \033[36m%-8s\033[0m %s\n", $$1, $$2}'
