# 更新日志

本文件记录 BRTEX 的变更。BRTEX 基于 ARTEX v0.3.15（闭源前可得的最后一个源码版本）继续维护，更早的上游版本历史不在本仓库维护。

## [Unreleased]

## [brtex-0.1.0] - 2026-10-27

### 新增

- `web_search` 新增 `bing`（cn.bing.com）与 `baidu` 两个无密钥爬取后端，国内网络可直连。
- LLM 配置与「获取模型列表」新增不可见 Unicode 清洗（零宽字符、BOM、word joiner 等），避免粘贴内容导致 URL/密钥解析失败。
- MCP 客户端在无文本块时回退读取 `structuredContent`（内置 `norma/mcp` 与远程 `mcphttp` 行为一致）。
- 任务归档在 `rename` 失败时回退为日志化复制，支持中断续跑与回滚。
- 补充归档复制回退、零宽字符清洗、进程树终止等测试。

### 变更

- LLM 熔断冷却梯度由 1m / 5m / 30m 调整为 30s → 1m → 1m → 2m → 5m → 10m，前端提示同步更新。
- Bash 工具在超时或取消时终止整棵进程树（Windows 使用 `taskkill /T`），避免残留子进程占用文件导致归档失败。
- Dockerfile：apt / npm 使用国内镜像并预置 `libllvm15`，修复无外网环境下的构建失败。
- 内置「版本与更新」改为检查 `ThanatosXingYu/BRTEX` 的发布。
- Docker 镜像默认本地构建，镜像名预留为 `ghcr.io/thanatosxingyu/brtex`。
- README 移除在线 Demo、截图预览与上游社区入口，补充本分支改动说明。

### 移除

- 上游作者/仓库的安装链接、镜像源与社区入口。
