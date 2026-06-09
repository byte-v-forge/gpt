# GPT

`gpt` 是 GPT 业务核心仓，承载公开账号能力、注册/登录/探测、Codex OAuth、typed action API 和公共编排 host。

## 核心能力

- 管理 GPT 账号库存、邮箱分配、任务状态和业务状态投影。
- 提供注册、登录、探测、Codex OAuth、OTP checkpoint 等可编排 action API。
- 作为公共 GPT runtime host，承接 n8n workflow 与 dashboard 的状态查询和动作入口。
- 提供插件 SPI，让私有 provider/action 通过 `gpt-private` 注册，而不侵入核心实现。
- 提供 GPT dashboard 模块，业务列表、动作区和详情展示保持数据驱动。

## 使用方式

公开流程和稳定契约放在本仓；私有 provider runtime、私有 workflow 和私有动作元数据放在 `gpt-private`。浏览器、邮箱、SMS、GoPay 等外部能力通过 proto/gRPC、HTTP、事件或部署配置集成。

## 入口

- 账号模块：`gpt-account/`
- 编排服务：`orchestrator/`
- 插件 SPI：`pkg/gptplugin/`
- 部署入口：`gpt-service/`
- 契约真源：`proto/`
- 前端模块：`webui/`

## 常用检查

```sh
./scripts/generate-proto.sh
(cd orchestrator && go list ./...)
(cd webui && npm run lint)
git diff --check
```
