# GPT

本仓承载 GPT 账号、注册、激活和支付相关实现。

## 目录

- `gpt-account/`：GPT 账号库存和邮箱分配数据模块。
- `gopay/`：GoPay App gRPC provider、GoPay payment gRPC provider 和钱包侧支付调用的 Go 协议实现。
- `stripe/`：ChatGPT checkout、Stripe payment pages 和 Midtrans Snap 支付链路的 Go 协议实现。
- `orchestrator/`：GPT 注册、登录、激活、GoPay 支付和任务状态服务；任务创建后通过 `platform-nats` 触发服务内 MQ worker 执行业务 action。
- `gpt-service/`：部署入口，把 GPT 账号库、GoPay 通道和 GPT workflow 组装为单个服务进程。
- `channels/gopay/whatsapp-relay/`：GoPay 场景使用的 Android OTP 通知转发器。
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

GPT job 以 PostgreSQL job projection 为状态真源；创建 job 时同事务写入 `gpt_platform_event_outbox`，outbox worker 发布 `gpt.job.action_requested` 到 `platform-nats`，再由 gpt-service 内置 action worker claim 对应 job 并执行。`ACTIVATE`、`AUTOPAY`、`PROBE_ACCOUNT`、`REGISTER_ACCOUNT`、`REGISTER_ACCOUNT_PROTOCOL`、`REGISTER_AND_ACTIVATE`、`LOGIN_SESSION`、`LOGIN_SESSION_PROTOCOL`、`CODEX_OAUTH`、`CODEX_OAUTH_PROTOCOL`、`CODEX_OAUTH_ADD_PHONE`、`CODEX_OAUTH_BATCH_ADD_PHONE`、`GOPAY_APP`、`GOPAY_PAYMENT`、`GOPAY_QRIS_PAYMENT_ACTIVATE`、`GOPAY_WA_PAYMENT` 和 `GOPAY_PAYMENT_REBIND` 走同一套 MQ worker。
MQ 只用于异步、可重试、跨边界的状态推进：job action 调度、SMS/邮箱验证码事件投影、邮箱 poll 唤醒请求。同步查询、手动取消、OTP 手动提交/重发、账号详情读取、运行时 secret/PIN/PKCE/GoPay token 状态不走 MQ，分别使用 gRPC/HTTP、PostgreSQL projection 或 Redis TTL store 保持直接一致性。
浏览器注册和登录通过 `BROWSER_AUTOMATION_ADDR` 连接 `browser-automation`，并使用 `BROWSER_AUTH_PROXY_REF`、`BROWSER_AUTH_LOCALE`、`BROWSER_AUTH_TIMEZONE` 和 `BROWSER_AUTH_WINDOW_WIDTH` / `BROWSER_AUTH_WINDOW_HEIGHT` 描述浏览器 profile；gpt-service 不再维护进程内 browser auth flow，业务 action runner 只在 `browser-automation` 持久 session 上推进到 result 或 OTP checkpoint，后续 complete/resend 通过 session id 继续执行。
邮箱验证码不再通过长等待 RPC 拉取 provider；`gpt-service` 通过 `PLATFORM_NATS_URL` 发布 `mailbox.email.poll_requested` 唤醒 mailbox worker，并消费 `sms.code.received` 与 `mailbox.email.received` 自行过滤解析 OTP，验证码只进入 Redis TTL cache，默认 5 分钟。GPT 侧不再直连 mailbox gRPC，邮箱池查询走 mailbox 自己的 `/api/mailbox` BFF。
GoPay webhook OTP 由 `gpt-service` HTTP 入口接收，`GOPAY_OTP_WEBHOOK_LISTEN_ADDR` 控制监听地址，`GOPAY_OTP_WEBHOOK_TTL_SECONDS` 和 `GOPAY_OTP_WEBHOOK_MAX_ITEMS` 控制 relay 生命周期和容量。
`CACHE_REDIS_URL` 是 GPT workflow 运行时依赖：GoPay webhook OTP relay 使用业务独立 Redis Stream/TTL 实现以支持多副本和服务重启；workflow PIN、PKCE、OAuth auth JSON 等临时敏感值使用同一业务 Redis 的 `GPT_RUNTIME_SECRET_KEY_PREFIX` 命名空间和 `GPT_RUNTIME_SECRET_TTL_SECONDS` 生命周期，不再写入 PostgreSQL。
GoPay app 登录 token、设备/代理指纹和 OTP pending 等 runtime state 使用同一 Redis 的 `GOPAY_STATE_KEY_PREFIX` 命名空间和 `GOPAY_STATE_TTL_SECONDS` 生命周期，不再创建 `gopay_app_states` PostgreSQL 表；PostgreSQL 只保留 job/account 等需要查询的持久投影。
GoPay 接码生命周期通过 `SMS_ADDR` 连接 `sms-service` 创建/标记/取消 activation；验证码到达后由 SMS 事件进入 GPT OTP 投影，目标固定为 GoPay 使用的印度尼西亚号码策略。
