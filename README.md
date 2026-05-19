# GPT

本仓承载 GPT 账号、注册、激活和支付相关实现。

## 目录

- `account-db/`：GPT 账号库存和邮箱分配数据服务。
- `registration/browser-reg/`：GPT 浏览器注册和登录自动化服务。
- `orchestrator/`：GPT 注册、登录、激活、GoPay 支付和任务状态 workflow。
- `channels/gopay/app/`：GoPay App 通道能力。
- `channels/gopay/payment/`：GoPay 支付通道能力。
- `channels/gopay/whatsapp-relay/`：GoPay 场景使用的 Android OTP 通知转发器。
- `channels/paypal/`：PayPal 通道目录。
- `proto/`：本仓服务使用的 proto 源契约。
- `docker/camoufox-base/`：浏览器注册服务基础镜像定义。

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
邮箱验证码和邮箱池读取通过 `MAILBOX_ADDR` 连接 `mailbox-api`。
