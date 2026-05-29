# GPT

本仓承载 GPT 账号、注册、登录、探测、Codex OAuth 和公共编排 host 能力。私有 GoPay provider runtime、私有动作元数据和私有 workflow 归 `gpt-private`。

## 目录

- `gpt-account/`：GPT 账号库存和邮箱分配数据模块。
- `orchestrator/`：GPT 注册、登录、探测、Codex OAuth 和任务状态服务；长流程统一由 n8n workflow 编排，服务只提供 typed action API 与状态投影。
- `pkg/gptplugin/`：GPT action/config/workflow 插件注册 SPI；不承载具体私有 provider 常量。
- `gpt-service/`：部署入口；基础镜像不包含私有 GoPay sidecar，私有目标由部署期显式选择。
- `proto/`：本仓服务使用的 proto 源契约。

## 生成

```bash
./scripts/generate-proto.sh
```

## 检查

```bash
./scripts/generate-proto.sh
(cd orchestrator && go list ./...)
(cd webui && npm run lint)
```

## 运行配置

GPT job 以 PostgreSQL job projection 为状态真源；长流程入口由 dashboard BFF 创建 job 后触发对应 n8n webhook。公开仓保留 `LOGIN_SESSION`、`LOGIN_SESSION_PROTOCOL`、`PROBE_ACCOUNT` 和 Codex OAuth 等核心动作；私有注册与 GoPay 动作由 `gpt-private` 通过 plugin 注册。gpt-service 不再启动内部执行 worker，也不再发布旧 action 调度事件。
MQ 只用于跨服务事件和投影推进：SMS/邮箱验证码事件投影、邮箱 poll 唤醒请求、hotstream/SSE 状态事件等。同步查询、手动取消、OTP 手动提交/重发、账号详情读取、运行时 secret/PIN/PKCE/GoPay token 状态不走 MQ，分别使用 gRPC/HTTP、PostgreSQL projection 或 Redis TTL store 保持直接一致性。
浏览器注册和登录通过 `BROWSER_AUTOMATION_ADDR` 连接 `browser-automation`，并使用 `browser_auth` plugin config 描述浏览器 profile；gpt-service 不再维护进程内 browser auth flow，n8n 只调用 typed action API 推进到 result 或 OTP checkpoint，后续 complete/resend 通过 session id 继续执行。
邮箱验证码不再通过长等待 RPC 拉取 provider；`gpt-service` 通过 `PLATFORM_NATS_URL` 发布 `mailbox.email.poll_requested` 唤醒 mailbox worker，并消费 `sms.code.received` 与 `mailbox.email.received` 自行过滤解析 OTP，验证码只进入 Redis TTL cache，默认 5 分钟。GPT 侧不再直连 mailbox gRPC，邮箱池查询走 mailbox 自己的 `/api/mailbox` BFF。
workflow PIN、PKCE、OAuth auth JSON 等临时敏感值使用业务 Redis 的 `GPT_RUNTIME_SECRET_KEY_PREFIX` 命名空间和 `GPT_RUNTIME_SECRET_TTL_SECONDS` 生命周期，不再写入 PostgreSQL。
