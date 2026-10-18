# syntax=docker/dockerfile:1

# ---------------------------------------------------------------- 构建阶段
FROM golang:1.27-alpine AS build

WORKDIR /src

# 项目零第三方依赖，go.mod 里只有 module 与 go 指令，不需要 go.sum
COPY go.mod ./
COPY . .

# CGO_ENABLED=0 → 静态链接，运行阶段可以放进任意精简基础镜像
RUN CGO_ENABLED=0 GOOS=linux go build \
        -trimpath \
        -ldflags "-s -w" \
        -o /out/localdrive .

# ---------------------------------------------------------------- 运行阶段
FROM alpine:3.21

# ca-certificates：favicon 支持 http(s):// 远程图片
# tzdata：让日志时间戳跟随 TZ 环境变量
RUN apk add --no-cache ca-certificates tzdata \
 && adduser -D -u 10001 -s /sbin/nologin localdrive

WORKDIR /app

COPY --from=build /out/localdrive /app/localdrive
# 容器内默认配置；挂载自己的 config.ini 到 /app/config.ini 即可覆盖
COPY deploy/config.docker.ini /app/config.ini

# TMPDIR 决定 appDir()：日志、state.json、单实例锁都会落在 /data/LocalDrive 下。
# 这个值必须固定，换掉就等于换了一套运行状态。
ENV TMPDIR=/data \
    TZ=Asia/Shanghai

# /data           持久化：日志 + 状态 + 发布临时目录
# /app/published  publishDir 留空时的兜底写入位置
# /share          默认展示目录，挂载宿主机目录到这里
RUN mkdir -p /data/published /app/published /share \
 && chown -R localdrive:localdrive /data /app/published /share

USER localdrive

EXPOSE 8080

# 端口被占用时程序会自动向后顺延，此时健康检查会报 unhealthy，但服务本身正常。
# 改了 [server] port 的话，同步设置 -e LOCALDRIVE_PORT=新端口。
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- "http://127.0.0.1:${LOCALDRIVE_PORT:-8080}/" >/dev/null || exit 1

ENTRYPOINT ["/app/localdrive"]
