# GPT

本仓承载 GPT 账号、注册、激活和支付相关实现。

## 目录

- `account-db/`：GPT 账号库存和邮箱分配数据服务。
- `orchestrator/`：GPT 注册、登录、激活、GoPay 支付和任务状态 workflow，并通过 `browser-automation` 执行浏览器注册和登录步骤。
- `channels/gopay/app/`：GoPay App 通道能力。
- `channels/gopay/payment/`：GoPay 支付通道能力。
- `channels/gopay/whatsapp-relay/`：GoPay 场景使用的 Android OTP 通知转发器。
- `channels/paypal/`：PayPal 通道目录。
- `proto/`：本仓服务使用的 proto 源契约。

## 生成

```bash
./scripts/generate-proto.sh
```

## 检查

```bash
./scripts/generate-proto.sh
go build ./...
```

Go 检查需要分别在 `account-db/` 和 `orchestrator/` 下执行。

## 运行配置

`orchestrator/` 通过 `TEMPORAL_ADDRESS`、`TEMPORAL_NAMESPACE`、`TEMPORAL_TASK_QUEUE` 和 `TEMPORAL_IDENTITY` 连接组织内 workflow runtime。
浏览器注册和登录通过 `BROWSER_AUTOMATION_ADDR` 连接 `browser-automation`，并使用 `BROWSER_AUTH_PROXY_REF`、`BROWSER_AUTH_LOCALE`、`BROWSER_AUTH_TIMEZONE` 和 `BROWSER_AUTH_WINDOW_WIDTH` / `BROWSER_AUTH_WINDOW_HEIGHT` 描述浏览器 profile。
邮箱验证码和邮箱池读取通过 `MAILBOX_ADDR` 连接 `mailbox-api`。
GoPay webhook OTP 由 orchestrator 进程内 HTTP 入口接收，`GOPAY_OTP_WEBHOOK_LISTEN_ADDR` 控制监听地址，`GOPAY_OTP_WEBHOOK_TTL_SECONDS` 和 `GOPAY_OTP_WEBHOOK_MAX_ITEMS` 控制内存队列生命周期。
接码生命周期通过 `SMS_ADDR` 连接 `sms-service`，并使用 `SMS_APPLICATION_KEY`、`SMS_COUNTRY_ISO2`、`SMS_COUNTRY_CALLING_CODE`、`SMS_MAX_PRICE_DECIMAL` 和 `SMS_POLL_INTERVAL_SECONDS` 描述 GPT/GoPay 所需目标。
