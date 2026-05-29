# AGENTS.md

本仓是 `gpt` 业务仓。

- 破坏性迁移按目标职责直接演进。
- 公共或跨服务契约以 `proto/` 为源头。
- 生成代码输出到各服务本地 `pb/` 或 Python `_pb2.py` 文件，提交前保持忽略。
- Go 服务优先通过 `./scripts/generate-proto.sh` 生成契约代码后做编译检查。
- `webui/` 下手写前端源文件单文件不得超过 200 行；超过时先拆成业务模块、hook、utils，并复用 `webui` 仓 module-kit/uikit。
- `webui/` 前端实现必须尽量减少手写代码；优先使用官方组件、官方 SDK、官方示例推荐模式，或轻量组装 `webui` 暴露的官方组件后复用。
- `webui/` 下不得手写基础 UI 组件；toolbar、tabs、table/list、sheet/dialog、icon button、empty state、copy field 等统一从 `webui` 共享组件引入。
- `webui/` 表单优先使用 React Hook Form 结合 shadcn/Radix 官方表单、输入、选择和校验组件；本仓只声明业务字段、默认值、提交参数和轻量布局。
- `webui/` 只能依赖 `webui` 共享组件、业务本仓代码，以及基础设施仓提供的通用组件/SDK/契约；不得依赖基础设施仓页面组件，也不得要求 `webui` 仓 import 本仓业务页面。
- GPT core 页面、业务数据请求、业务动作和详情抽屉归本仓；最终装载由 `deploy` 的声明式前端组合完成。
- GoPay 等私有 provider runtime、action metadata、私有配置 schema 和私有 workflow 归 `gpt-private`；Codex 等可公开归属的 GPT 动作可保留在本仓；本仓保留 GPT runtime host、公共 GPT 领域核心、公开契约和迁移中的 host-side 编排/原子 API。
- GPT core 后端 workflow、activity、状态机、动作条件和数据模型归本仓；基础设施仓不得感知这些业务类型。
- 业务能力差异在契约或 registry 中声明，例如渠道、支付方式、OTP 方式、动作参数、required fields、required statuses 和可重试条件；后端校验和前端渲染复用同一语义，不各自硬编码。
- 前端优先使用数据驱动组件；业务列表、动作区和详情通过 columns/actions/capabilities/render hooks 配置 `webui` 或基础设施仓通用组件，不把业务分支塞进基础组件。
- 渠道、provider 或业务资源能力差异使用声明式 capabilities/actions/required fields/required statuses 表达；资源项只保留自身状态和业务字段，不重复携带可由 provider/业务类型声明推导的能力列表。
- 不同渠道或 provider 需要不同交互时拆成独立页面组件文件，禁止把多个 provider/渠道 UI 揉在一个大组件里。
- 前端查询统一使用 TanStack Query；SSE/事件推送通过共享事件适配层进入 QueryClient cache 或本仓领域 hook。
- 第三方 provider 已有官方 SDK、官方 UI/Web Component、官方图标或官方设计资产时，优先使用官方维护包；不得手写等价组件或伪品牌图标。
- 本仓不维护测试文件、测试目录、测试夹具、覆盖率报告或 CI/CD 配置。
- 运行时配置来自环境变量、配置文件或部署配置。
- secret、token、cookie、验证码、支付凭据、代理凭据和会话材料按敏感数据处理。
