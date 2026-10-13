# Stora 服务端镜像。
#
# 只打包 Go 二进制（API / WebDAV / WOPI）。前端是纯静态产物，按现有部署文档
# （docs/deployment-nginx.md）由 Nginx 或任意静态托管提供，不进这个镜像——
# 服务端代码里没有任何静态文件服务逻辑。
#
# 构建：docker build -t stora .
# 运行：docker run -p 9421:9421 --env-file .env -v /srv/stora/storage:/app/storage stora

# 基础镜像版本可用 --build-arg GO_VERSION=1.25 覆盖：
# 镜像仓库的标签偶尔会调整，留一个不改文件就能回退的口子。
ARG GO_VERSION=1.26
FROM golang:${GO_VERSION}-alpine AS build
WORKDIR /src

# 依赖单独一层：go.mod / go.sum 不变时不会重复下载
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# CGO_ENABLED=0 产出静态二进制，运行时才敢用 alpine（不带 glibc）
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/stora-server ./cmd/server \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/stora-cli ./cmd/cli

FROM alpine:3.20
# ffmpeg: 视频缩略图与转码；ca-certificates: 离线下载 / Webhook 的外呼 HTTPS
RUN apk add --no-cache ca-certificates tzdata ffmpeg

WORKDIR /app
COPY --from=build /out/stora-server /out/stora-cli /usr/local/bin/
COPY migrations ./migrations
COPY config ./config

# 内容寻址对象根目录与分片上传临时目录——部署时挂载出来，别留在容器层里
VOLUME ["/app/storage", "/app/temp"]
EXPOSE 9421
ENTRYPOINT ["stora-server"]
