# Stripe

本目录承载 ChatGPT checkout、Stripe payment pages 和 Midtrans Snap 支付链路使用的 Go 协议实现。

## 目录

- `client.go`：ChatGPT、Stripe 和 Midtrans 的协议客户端集合。
- `credential.go`：ChatGPT session token 与 access token 的单一凭据模型。
- `internal/protocol/`：本目录协议客户端使用的 HTTP、JSON 和日志脱敏工具。

## 检查

```bash
go build ./...
```
