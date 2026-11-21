# syntax=docker/dockerfile:1

# 全局 ARG：必须在文件最顶部声明，才能在所有 FROM 中使用
ARG GO_VERSION=1.27

# ---- 阶段 1：构建前端 ----
FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# ---- 阶段 2：Go 静态编译（纯 Go SQLite，CGO 关闭）----
FROM golang:${GO_VERSION}-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/web/dist ./web/dist
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath \
      -ldflags "-s -w -X github.com/SinGyoZ/TarShip/internal/version.Version=${VERSION}" \
      -o /out/tarship ./cmd/tarship \
 && mkdir -p /out/data

# ---- 阶段 3：运行镜像（scratch + CA 证书，体积最小）----
FROM scratch
# 预先创建 /data 并交给 65532（nonroot）：Hub 以 nonroot 运行时，命名卷首次创建会继承这个属主。
COPY --from=build --chown=65532:65532 /out/data /data
COPY --from=build /out/tarship /usr/local/bin/tarship
# golang:alpine 阶段自带 CA 证书；Agent 发起 wss:// 连出依赖它
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
ENV TARSHIP_DATA=/data
VOLUME /data
EXPOSE 8080
# 同一镜像既能跑 Hub 也能跑 Agent：默认 Hub，Agent 用 `command: agent`。
# 镜像默认以 root 运行（Agent 读取与恢复任意目录需要）；Hub 建议在 compose 里指定 user: "65532:65532"。
ENTRYPOINT ["/usr/local/bin/tarship"]
CMD ["hub"]
# healthcheck 是二进制内置子命令，scratch 无 shell 也能跑
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD ["/usr/local/bin/tarship", "healthcheck"]
