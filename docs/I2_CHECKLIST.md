# I2 详细执行计划：Runtime 扩展版

> 来源：`docs/INDUSTRIAL_ROADMAP.md`  
> 状态：Draft  
> 依赖：I1 完成并通过退出评审  
> 说明：本计划只描述逻辑变更、组件边界、执行顺序和验收，不展开函数、文件和数据库字段级实现。

## 0. I2 目标与边界

### 目标

在保持简单 Runtime 写法不变的前提下，让 Runtime 可以承载更复杂的执行模型：

- Runtime Driver；
- 能力声明；
- 自定义事件；
- Runtime-local 工具；
- PostgreSQL 内 checkpoint / resume；
- LangGraph 原型。

### 边界

I2 仍然保持：

- 一个 Runtime 进程只承载一个 Runtime 定义；
- 默认单实例，可选同定义多副本；
- PostgreSQL 是唯一状态源；
- 不引入 Redis、NATS、对象存储；
- 不做 Sandbox 生产化；
- 不做控制面多副本；
- 不实现 App Bridge v2。

## 1. 进入条件

- [ ] I1 已合并或已通过退出评审。
- [ ] 简单 Echo Runtime 和 React Runtime 测试通过。
- [ ] 当前 RuntimeChannel v1 契约稳定。
- [ ] PostgreSQL 迁移基线稳定，I1 的租户字段已存在。
- [ ] 明确 I2 不引入新的基础设施。
- [ ] 明确 I2 不修改简单 `run(ctx)` 的外部写法。

## 2. 工作流 A：契约扩展

### 逻辑变更

在 v1 上做增量扩展，不删除现有语义：

- Runtime 能力声明；
- 自定义事件；
- checkpoint 元数据；
- waiting / resume；
- 父子 Run；
- Runtime-local 工具来源。

### 引入组件

- Capability Manifest
- Custom Event Envelope
- Checkpoint Metadata
- Resume Contract
- Run Parent/Child Relation
- Tool Source Descriptor

### 实现方法

1. 先冻结字段语义，再生成 Go/Python stub。
2. 所有新增字段保持向后兼容。
3. 旧 Runtime 不声明能力时按最小能力集运行。
4. 自定义事件必须有名称和 schema 版本。
5. checkpoint 必须携带 codec、版本和 watermark。
6. resume 必须绑定 Run、Session、租户和租约。
7. 更新协议契约文档和 conformance 用例。

### 验收

- 旧 Echo/React Runtime 不需要修改。
- 旧客户端缺少新字段时仍可运行。
- 新字段可被 Go 控制面和 Python Runtime 同时识别。
- `buf breaking` 通过。
- 契约文档与权威路线图一致。

## 3. 工作流 B：Runtime Driver 与能力

### 逻辑变更

在简单入口之外增加高级 Runtime 形态：

- Runtime 可以声明自身能力；
- 可以管理启动、关闭、重连和取消；
- 可以提供 checkpoint 编解码；
- 可以注册 Runtime-local 工具；
- 可以声明自定义事件；
- 同一进程仍然只承载一个 Runtime 定义。

### 引入组件

- Runtime Driver
- Runtime Capability Manifest
- Lifecycle Hooks
- Checkpoint Codec
- Runtime Tool Registry
- Provider Registry

### 实现方法

1. 保留现有简单入口。
2. Driver 作为可选组合层，不替换底层生命周期。
3. 能力声明在握手阶段上报。
4. 控制面只向 Runtime 发送其声明支持的能力。
5. 生命周期钩子覆盖启动、关闭、重连和取消。
6. checkpoint 编解码由 Runtime 提供，存储由控制面负责。
7. Runtime-local 工具先进入统一工具清单，再参与模型调用。

### 验收

- 现有简单 Runtime 完全不受影响。
- 自定义 Driver 可以运行和停止。
- Driver 可以声明能力并被控制面识别。
- 不支持某能力时自动降级。
- Runtime 作者仍然不需要接触传输协议和 protobuf。

## 4. 工作流 C：自定义事件

### 逻辑变更

Runtime 可以表达固定事件之外的执行语义：

- 图节点开始/结束；
- 状态机迁移；
- 子任务；
- 工具和模型调用跨度；
- 自定义业务事件。

### 引入组件

- Custom Event API
- Event Schema Registry
- Event Adapter
- Event Replay

### 实现方法

1. 固定事件继续保留。
2. 自定义事件使用通用事件类型加事件名称。
3. 事件名称建议命名空间化，例如 `graph.node.started`。
4. 事件包含 schema 版本，控制面按版本持久化。
5. 持久事件参与顺序和回放，ephemeral 事件不落库。
6. App WebSocket 和事件查询接口可以看到自定义事件。
7. 观测系统按事件名称聚合，不依赖 Runtime 类型。

### 验收

- 自定义事件可持久化。
- 断线重连后可以回放。
- 事件顺序和租户隔离正确。
- 旧固定事件的语义和顺序不变。
- 事件名称和 schema 版本可追踪。

## 5. 工作流 D：Runtime-local 工具与 Provider

### 逻辑变更

工具来源扩展为：

```text
App local
global
MCP
Runtime-local
```

Runtime 可以注册自己的工具，并参与模型工具清单。

### 引入组件

- Tool Source Descriptor
- Runtime Tool Registry
- Tool Router
- Provider Registry

### 实现方法

1. 工具清单增加来源标识。
2. 模型只看到统一工具清单。
3. 路由按来源决定执行方。
4. Runtime-local 工具不经过 App Gateway。
5. Provider Registry 支持自定义模型提供方。
6. 工具调用保持 invocation 标识和幂等语义。
7. 工具进度和取消仍留给后续版本。

### 验收

- Runtime 可以注册工具。
- 模型可以调用本地、全局、MCP 和 Runtime-local 工具。
- 工具来源不会泄漏到模型语义。
- 不支持的来源被明确拒绝。
- 现有工具调用行为不变。

## 6. 工作流 E：Session、Checkpoint 与 Resume

### 逻辑变更

checkpoint 从“传输消息”变成 I2 的真实能力：

- Session 可以保存状态；
- Runtime 可以保存 checkpoint；
- 可以从 checkpoint 恢复；
- 支持等待外部输入后恢复；
- checkpoint 有版本和编解码器；
- I2 只存 PostgreSQL，不转对象存储。

### 引入组件

- Session Store
- Checkpoint Store
- Codec Registry
- Resume Coordinator
- Waiting State

### 实现方法

1. 控制面负责 Session 和 checkpoint 持久化。
2. Runtime 只提供状态编码和恢复逻辑。
3. checkpoint 与事件 watermark 关联。
4. 恢复时校验 Runtime、Session、租户、版本和租约。
5. 增加 waiting / resumed 的最小状态流转。
6. 增加 resume 入口，支持人工或外部输入恢复。
7. 过期、清理和保留策略在 I2 先做基础版本。
8. 大状态和对象存储留到 I4。

### 验收

- checkpoint 可以保存、查询和恢复。
- 版本不匹配时明确失败。
- 租户或 Session 不匹配时明确失败。
- waiting 状态可以恢复。
- 恢复后事件顺序和幂等语义正确。
- 没有引入对象存储或额外基础设施。

## 7. 工作流 F：LangGraph 适配

### 逻辑变更

LangGraph 作为可选 Runtime Driver 接入，不进入 Runtime SDK 核心。

### 引入组件

- LangGraph Driver
- Graph Event Adapter
- Checkpoint Adapter
- Interrupt/Resume Adapter

### 实现方法

1. 独立可选包，不增加核心依赖。
2. 图节点开始/结束转换为自定义事件。
3. 图状态通过 Codec 存入 Session/Checkpoint Store。
4. interrupt 转换为 waiting。
5. resume 通过统一 Resume 入口恢复。
6. 子图先支持同 Run 内表达；父子 Run 作为可选能力。
7. 工具节点走统一工具路由。
8. 模型节点走统一 Provider Registry。

### 验收

- 一个最小 LangGraph 可以运行。
- 节点事件可查询和回放。
- interrupt 后可以恢复。
- checkpoint 恢复后图状态正确。
- 不安装 LangGraph 时核心 Runtime SDK 不受影响。

## 8. 执行顺序

```text
I2.0 契约冻结
  -> I2.1 Runtime Driver 与能力声明
  -> I2.2 自定义事件
  -> I2.3 Runtime-local 工具与 Provider
  -> I2.4 Session / Checkpoint / Resume
  -> I2.5 LangGraph 原型
  -> I2.6 加固与退出评审
```

### I2.0 契约冻结

交付：

- Capability Manifest；
- Custom Event 语义；
- Checkpoint Metadata；
- Waiting/Resume 语义；
- Parent/Child 语义；
- 兼容性规则。

退出：

- 所有新增字段只增不改；
- 旧 Runtime 可以按最小能力运行。

### I2.1 Runtime Driver

交付：

- Driver 抽象；
- 能力声明；
- 生命周期行为；
- 简单入口保持不变。

退出：

- 简单 Runtime 无改动；
- 自定义 Driver 可运行。

### I2.2 自定义事件

交付：

- 事件 API；
- 事件持久化；
- 事件回放；
- 事件观测。

退出：

- 图节点事件可查询；
- 断线重连可回放。

### I2.3 Runtime-local 工具

交付：

- 工具来源；
- 工具注册；
- 统一路由；
- Provider Registry。

退出：

- Runtime 可以注册并调用自己的工具。

### I2.4 Checkpoint / Resume

交付：

- Session Store；
- Checkpoint Store；
- Codec Registry；
- waiting / resume。

退出：

- 可以保存和恢复；
- 版本和租户校验通过。

### I2.5 LangGraph

交付：

- 可选适配包；
- 图事件；
- 图 checkpoint；
- interrupt/resume。

退出：

- 最小图可以中断和恢复；
- 不污染核心 SDK。

### I2.6 加固

交付：

- 指标；
- 日志；
- 数据库迁移；
- 集成测试；
- 文档更新。

退出：

- CI 通过；
- PostgreSQL 集成测试通过；
- I2 checklist 完成。

## 9. 测试矩阵

| 场景 | 必须验证 |
|---|---|
| 旧 Echo Runtime | 不需要修改，行为不变 |
| 旧 React Runtime | 不需要修改，行为不变 |
| Driver 启动/关闭 | 生命周期正确 |
| 能力声明 | 不支持时降级 |
| 自定义事件 | 持久化、顺序、回放、租户 |
| Runtime-local 工具 | 注册、路由、调用、错误 |
| checkpoint 保存 | 版本、watermark、租户 |
| checkpoint 恢复 | 成功、版本不匹配、Session 不匹配 |
| waiting / resume | 恢复、取消、超时 |
| LangGraph | 节点事件、interrupt、resume |
| 数据库迁移 | 新库和旧库都能升级 |
| 兼容性 | v1 客户端仍可运行 |

## 10. 指标与可观测性

新增或扩展：

```text
levier_runtime_capabilities
levier_custom_events_total
levier_checkpoints_total
levier_checkpoint_size_bytes
levier_checkpoint_restore_total
levier_run_waiting_seconds
levier_run_resume_total
```

要求：

- 所有指标带租户维度；
- 事件名称和 schema 版本可追踪；
- checkpoint 失败和版本冲突可告警；
- waiting 时长可观测；
- 不引入新的指标后端。

## 11. 关键依赖与风险

1. **Checkpoint 存储先于 LangGraph**：没有持久化和恢复，interrupt/resume 没有意义。
2. **自定义事件先于图事件**：否则 LangGraph 只能复用固定事件。
3. **能力协商先于 Driver**：控制面必须知道 Runtime 支持什么。
4. **协议兼容风险**：新增字段必须只增不改。
5. **checkpoint 大小风险**：I2 只适合小状态，大状态留到 I4。
6. **resume 权限风险**：必须校验租户、Session、Run 和租约。
7. **LangGraph 依赖风险**：必须是可选包，不进入核心 SDK。

## 12. 待拍板事项

建议在 I2.0 契约冻结前确认：

1. 自定义事件是否使用统一 `CUSTOM` 类型加事件名称。
2. checkpoint 是否先统一为 JSON 编码，后续再扩展二进制 codec。
3. waiting 状态是否需要单独的对外 API，还是复用现有 Run 接口。
4. 父子 Run 是否在 I2 实现，还是只预留字段。
5. LangGraph 子图先同 Run 表达，还是直接创建子 Run。
6. I2 的 checkpoint 保留策略和最大大小。

## 13. I2 明确不做

- Sandbox 生产化。
- 对象存储。
- Redis / NATS。
- 控制面多副本。
- Runtime Gateway / App Gateway。
- App Bridge v2 实现。
- 多语言 SDK / Sidecar。
- 物理租户隔离。
- 完整 SaaS 计费。

## 14. 完成标记

- [ ] I2.0 契约冻结完成。
- [ ] Runtime Driver 和能力声明完成。
- [ ] 自定义事件完成。
- [ ] Runtime-local 工具和 Provider 完成。
- [ ] Session / Checkpoint / Resume 完成。
- [ ] LangGraph 原型完成。
- [ ] PostgreSQL 集成测试完成。
- [ ] 指标、日志和文档完成。
- [ ] I2 退出评审通过。
- [ ] 下一版本 I3 的范围确认。

## 15. 交付物

```text
契约：Capability Manifest、Custom Event、Checkpoint、Resume
Runtime：Driver、能力声明、自定义事件、Runtime-local 工具
控制面：Session Store、Checkpoint Store、Resume、等待状态
适配：LangGraph Driver 原型
验证：集成测试、迁移测试、兼容性测试
文档：I2 checklist、契约文档、运行说明
```

## 16. 当前进展

分支：`feat/i2-runtime-driver`

| 项目 | 状态 | 说明 |
|---|---|---|
| Runtime Driver 抽象 | 已完成 | 简单 `run(ctx)` 保持兼容 |
| Runtime Capabilities | 已完成 | SDK 元数据、Hello 握手、daemon 注册 |
| Driver 生命周期 | 已完成 | 启动、关闭 Hook |
| Driver 测试 | 已完成 | 简单入口和 Driver 都通过 |
| I2.0 协议契约 | 已完成 | 能力握手、自定义事件、checkpoint、waiting/resume 已落地 |
| 自定义事件 | 已完成 | proto、SDK、控制面持久化、API/WS、测试 |
| Checkpoint Store | 已完成 | PostgreSQL 保存、读取、租户归属校验 |
| Checkpoint 自动恢复 | 已完成 | 新 Run 加载同 Session 的最新 checkpoint |
| Session 自动创建 | 已完成 | longterm Run 缺省时生成 session_id |
| Session ID 返回 | 已完成 | Run 响应和 RunView 可读 |
| Session 归属校验 | 已完成 | App/租户不匹配时拒绝 |
| SDK Session 透传 | 已完成 | runtime_type、session_id 可传入 |
| Session 管理 API | 待开始 | 列表、关闭、清理、过期 |
| Waiting 状态 | 已完成 | 仅 longterm；要求已保存 checkpoint |
| Waiting / Resume API | 已完成 | `POST /api/v1/runs/{id}/resume`，恢复后重新入队 |
| Waiting SDK | 已完成 | `return await ctx.wait(state)`，`resume_run` 已透传 |
| Waiting 时长/恢复指标 | 待开始 | `levier_run_waiting_seconds` 等 |
| Provider Registry | 已完成 | 复用控制面 `model_providers`；自定义 provider 通过 adapter 解析，Runtime 可声明 `providers` |
| Runtime-local 工具 | 已完成 | `ctx.register_tool` / `@ctx.tool`，`scope=runtime`，直接在本进程执行 |
| Runtime-local 统一清单 | 已完成 | 与 App/global/MCP 工具一起进入 `ctx.tools` 和 toolbox |
| Runtime-local 路由 | 已完成 | 不经过 App Gateway；与控制面工具重名明确拒绝 |
| Runtime-local 调用观测 | 已完成 | `TOOL_CALL_BEGIN/END` 带 `tool_scope=runtime`，invocation 透传 |
| LangGraph 原型 | 待开始 | 依赖前序能力 |
