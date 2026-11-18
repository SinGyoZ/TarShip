# syntax=docker/dockerfile:1

# 多阶段构建：前端 node 阶段 → Go 静态编译 → scratch 极简运行层。
# 最终镜像只含：tarship 二进制 + CA 根证书（供 agent 模式发起 wss 连出）。
ARG GO_VERSION=1.27
ARG NODE_VERSION=24

# ---- 阶段 1：构建面板 ----
FROM node:${NODE_VERSION}-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

# ---- 阶段 2：Go 静态编译 ----
FROM golang:${GO_VERSION}-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
# 只拷贝 embed 声明与构建产物，避免把 web/node_modules 拉进镜像构建上下文
COPY web/embed.go ./web/embed.go
COPY --from=web /src/web/dist ./web/dist
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
    -ldflags="-s -w -X github.com/SinGyoZ/TarShip/internal/version.Version=${VERSION}" \
    -o /out/tarship ./cmd/tarship

# ---- 阶段 3：scratch 运行层 ----
FROM scratch
LABEL org.opencontainers.image.title="TarShip" \
      org.opencontainers.image.description="目录备份运输系统：Hub 控制面 + Agent 直传存储器" \
      org.opencontainers.image.source="https://github.com/SinGyoZ/TarShip" \
      org.opencontainers.image.licenses="MIT"
COPY --from=build /out/tarship /tarship
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
VOLUME ["/data"]
EXPOSE 8080
# 同一镜像双角色：默认 hub；Agent 用 `command: agent --server ... --token ...`
ENTRYPOINT ["/tarship"]
CMD ["hub"]
