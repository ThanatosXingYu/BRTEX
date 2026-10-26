# syntax=docker/dockerfile:1
#
# 多阶段构建：前端静态导出 → Go 单二进制（内嵌前端）→ 运行镜像。
# 原 Docker Hub 镜像源（autumn27/artex）已失效，本仓库自此支持从纯源码自助构建：
#   docker compose build        # 或 docker build -t artex:local .
# TARGETARCH 由 buildx 自动注入（amd64/arm64），Go 交叉编译无需 QEMU；
# 运行层只装工具，编译全部在构建阶段完成。
# 国内网络可覆盖模块源（默认已对国内友好）：
#   --build-arg GOPROXY=https://goproxy.cn,direct
#   --build-arg NPM_REGISTRY=https://registry.npmmirror.com

########## 阶段 1：前端静态导出 ##########
FROM node:20-bookworm AS web
ARG NPM_REGISTRY=https://registry.npmmirror.com
WORKDIR /web
# --ignore-scripts：跳过 prepare(husky)——容器里没有 .git，husky 会失败；
# Next.js/Biome 的平台二进制走 optionalDependencies，不依赖生命周期脚本。
COPY web/package.json web/package-lock.json ./
RUN npm config set registry "$NPM_REGISTRY" && npm ci --ignore-scripts
COPY web/ ./
RUN NEXT_EXPORT=1 npm run build:static

########## 阶段 2：Go 后端（内嵌前端，静态编译）##########
FROM golang:1.26-bookworm AS gobuild
ARG TARGETARCH
ARG GOPROXY=https://goproxy.cn,https://proxy.golang.org,direct
WORKDIR /src
COPY go.mod go.sum ./
# go.mod 有 replace github.com/Autumn-27/norma => ./norma，解析模块图必须先看到
# ./norma/go.mod，否则 go mod download 直接失败。只需模块文件，源码留给下面 COPY . .
COPY norma/go.mod norma/go.sum ./norma/
RUN go mod download
COPY . .
# 前端静态产物从 web 阶段拷入（.dockerignore 已排除宿主 dist，不会冲突）
COPY --from=web /web/out/ server/webui/dist/
RUN CGO_ENABLED=0 GOARCH="${TARGETARCH}" go build -tags embedui -trimpath -o /out/artex ./cmd/artex

########## 阶段 3：运行镜像（同原版：工具 + Playwright 预装）##########
FROM python:3.12-slim-bookworm
ARG NPM_REGISTRY=https://registry.npmmirror.com
# 运行层的全局 npm 装包与 chromium 下载同样走国内源（对全球网络同样可达）
ENV npm_config_registry=$NPM_REGISTRY \
    PLAYWRIGHT_DOWNLOAD_HOST=https://cdn.npmmirror.com/binaries/playwright
# apt 源切国内镜像：直连 deb.debian.org 在国内网络下又慢又断（libllvm15 反复
# Connection failed 导致构建失败），playwright --with-deps 内部还会再跑 apt，同样受益。
RUN sed -i 's|deb.debian.org|mirrors.aliyun.com|g' /etc/apt/sources.list.d/debian.sources
# 常用工具：ripgrep / curl / vim，加一批 recon 常备件（按需增删）。
# Node 从 NodeSource 装 20.x：bookworm 自带的 apt nodejs 是 18，Playwright 要求 >=20。
RUN apt-get update && apt-get install -y --no-install-recommends \
      ca-certificates ripgrep curl wget vim git jq unzip \
      dnsutils iputils-ping netcat-openbsd inetutils-telnet whois nmap \
    && curl -fsSL https://deb.nodesource.com/setup_20.x | bash - \
    && apt-get install -y --no-install-recommends nodejs \
    && rm -rf /var/lib/apt/lists/*
# 预装 libllvm15：它是 chromium 依赖链里最大的包（23MB），国内网络下 apt 客户端
# 下载必被重置（curl/urllib 直连却通）。这里用 urllib 下载后以本地 .deb 路径直接
# 安装，完全绕开 apt 联网拉取；包一旦装好，playwright --with-deps 内部的
# apt-get install 会跳过它。文件名与 URL 用 --print-uris 动态解析，版本变化自动跟随。
RUN apt-get update \
    && python3 - <<'EOF'
import re, subprocess, urllib.request, time
out = subprocess.run(["apt-get", "download", "--print-uris", "libllvm15"],
                     capture_output=True, text=True, check=True).stdout
m = re.match(r"'([^']+)'\s+(\S+)\s+(\d+)", out)
if not m:
    raise SystemExit("libllvm15 解析失败: " + out)
url = m.group(1)
for attempt in range(5):
    try:
        urllib.request.urlretrieve(url, "/var/cache/apt/archives/libllvm15.deb")
        break
    except Exception as e:
        print("attempt", attempt, "failed:", e)
        time.sleep(3)
else:
    raise SystemExit("libllvm15 预置失败")
subprocess.run(["apt-get", "install", "-y", "--no-install-recommends",
                "/var/cache/apt/archives/libllvm15.deb"], check=True)
EOF
# 预装 Playwright MCP 与 CLI（全局），运行时不再 npx 联网下载。
# @playwright/mcp：browser MCP 直接 `npx @playwright/mcp`（已全局装好，无需 -y/@latest）。
# @playwright/cli：提供 playwright-cli，装完顺带 --help 验证可执行。
# 再装 playwright（提供浏览器管理），装完用 --with-deps 预置 chromium 及其系统依赖，
# 这样容器内 MCP/CLI 首次启动即可用，不再联网下载浏览器。
RUN npm install -g @playwright/mcp@latest @playwright/cli@latest playwright@latest \
    && playwright-cli --help \
    && playwright install --with-deps chromium \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
# 构建阶段产出的单二进制（内嵌前端）
COPY --from=gobuild /out/artex /app/artex
# 守护启动脚本：进程退出后按退出码决定是否重新拉起，页面一键更新靠它完成换装。
# 它同时负责把 SIGTERM 转发给 artex —— docker stop 只把信号发给 PID 1，
# 不转发的话 artex 收不到、做不了优雅关闭，10 秒后被 SIGKILL 硬杀。
COPY start.sh /app/start.sh
RUN chmod +x /app/artex /app/start.sh
COPY skills/ /app/skills/
# data/（SQLite + jwt.key）持久化点
VOLUME ["/app/data"]
EXPOSE 8787 8788
ENTRYPOINT ["/app/start.sh"]
CMD ["-addr", ":8787", "-proxy", ":8788"]
