<!-- Levier — adapter and sidecar roadmap (deferred, prioritized). -->

# Adapter / Sidecar 路线图

| 项目 | 内容 |
|---|---|
| 状态 | Python SDK C0/C1 第一版已实现；Sidecar 与其他语言 SDK 延后 |
| 归属 | 产品能力，当前只支持 Python App/runtime |
| 目的 | 降低 App 接入成本，覆盖更多语言和部署形态 |
| 前置 | 控制面可操作面稳定（CLI / 配置 / 自描述 / 状态 / 自检） |

---

## 1. 结论

当前先做 Python adapter；Sidecar、Node/TypeScript、Go SDK 继续延后。

顺序建议：

```text
P0  凭据文件 + app init                        已完成
P0  doctor --app / conformance                 部分完成
P1  Python SDK（C0/C1）                         第一版已实现
P1  Sidecar + manifest                         延后
P1  tool test / replay                         延后
P2  Node / TypeScript SDK                      延后
P2  OpenAPI 生成客户端 / Go SDK                 延后
P3  GUI onboarding wizard                      延后
```

Python SDK 的第一版产物在 `sdk/python/`，契约见
`docs/APP_ADAPTER_CONTRACT.md`。当前明确不修改 Harness、MCP Runtime 或其他
运行侧组件；先把 App 接入代码收敛为业务 handler、工具 schema 和凭据。

---

## 2. 评估标准

| 维度 | 问题 |
|---|---|
| 利益 | 能减少多少接入代码、多少支持工单、覆盖多少用户 |
| 代价 | 开发时间、维护语言数量、发布与测试矩阵、部署复杂度 |
| 风险 | 是否引入新的进程、新的安全边界、新的故障模式 |
| 前置 | 依赖哪些控制面能力先稳定 |

判断一个 Adapter 值不值得做，只看一个指标：

> **一个新 App 从零到跑通，能不能在 30 分钟内完成，并且业务代码里不出现 WebSocket、token 刷新、重连和幂等。**

---

## 3. 优先级总表

| 优先级 | 项 | 利益 | 代价 | 前置条件 |
|---|---|---|---|---|
| P0 | 凭据文件 + `app init` | 接入方不再手写 token 存储；降低凭据泄露风险 | 低，1–2 天 | App 认证与轮换稳定 |
| P0 | `doctor --app` / conformance | 文档不再靠猜；支持成本明显下降 | 中，3–5 天 | `/meta`、基础 doctor 已存在 |
| P1 | Python SDK | 参考实现和 Python 应用接入 ≤ 30 分钟 | 中，约 1 周 | 契约稳定、凭据文件可用 |
| P1 | Sidecar + manifest | 已有 HTTP 服务零侵入接入 | 中高，1–2 周 | 本地 handler 协议、进程管理 |
| P1 | `tool test` / replay | 工具调用可单独调试，减少联调时间 | 低中，2–3 天 | 调用记录、WebSocket 稳定 |
| P2 | Node / TypeScript SDK | 覆盖 Node / 前端团队 | 中，约 1 周 | Python SDK 验证过抽象 |
| P2 | OpenAPI 生成客户端 | 降低多语言客户端维护成本 | 低中，3–5 天 | OpenAPI 完整、契约测试稳定 |
| P2 | Go SDK | 覆盖 Go 服务 | 中，约 1 周 | Go 客户端抽象稳定 |
| P3 | GUI onboarding wizard | 演示、低代码用户友好 | 高，2–3 周 | CLI / doctor / Adapter 都已稳定 |

---

## 4. 各项说明

### P0：凭据文件 + `app init`

解决“接入方要自己接受并保存鉴权 token”的问题。

产物：

```text
levier app init --name <app> --credentials-out <path>
0600 凭据文件：app_id / app_name / api_key
levier app rotate-key --write-credentials
```

收益：

```text
接入方不接触 Admin token
不需要自己设计凭据文件格式
轮换 key 有标准动作
```

代价与风险：

```text
文件权限、原子写入、跨平台路径
轮换期间的短暂 401
```

### P0：`doctor --app` / conformance

解决“照着文档改仍然改不对”的问题。

产物：

```text
levier doctor --app <path>
levier conformance --app <path>
```

检查：

```text
凭据可用
工具注册与本地声明一致
WebSocket 能建立
ping / pong 正常
真实工具调用能往返
事件能订阅与补拉
```

收益：

```text
接入方自己就能定位哪一跳坏了
支持工单从“猜”变成“看输出”
```

代价与风险：

```text
需要维护一套契约用例
需要处理不同语言/运行时的差异
```

### P1：Python SDK（C0/C1 第一版已实现）

第一个正式 SDK，优先服务参考实现和 Python 团队。

产物：

```text
sdk/python/（包名 levier-app，import levier_app）
注册工具 handler
SDK 负责 HTTP、ws_token、WebSocket、重连、结果序列化
```

收益：

```text
接入代码从几百行降到几十行
协议升级只改 SDK
```

代价与风险：

```text
SDK 版本与协议版本要对齐
SDK 内部重连/并发/背压需要测试
不能把业务逻辑吸进 SDK
```

### P1：Sidecar + manifest

面向已有 HTTP 服务的零侵入方案。

产物：

```text
工具 manifest：tool name → 本地 HTTP endpoint
sidecar：持有 App key、WebSocket、重连、token 刷新
App：只增加本地 handler
```

收益：

```text
任何语言都能接，不需要 SDK
业务进程不需要理解 WebSocket
适合遗留系统和多语言团队
```

代价与风险：

```text
多一个部署单元和进程生命周期
sidecar 与 App 的本地鉴权
日志与故障排查多一层
需要明确 sidecar 不持有 Admin token
```

### P1：`tool test` / replay

调试工具调用，不必每次跑完整 Run。

产物：

```text
levier tool test --name <tool> --args '<json>'
levier run replay <run_id>
```

收益：

```text
工具问题可以单独复现
减少“模型不调用”与“工具执行失败”的混淆
```

### P2：Node / TypeScript SDK

在 Python SDK 验证抽象后再做，避免同时维护两套不成熟设计。

### P2：OpenAPI 生成客户端 / Go SDK

优先用 OpenAPI 生成基础 HTTP 客户端，再决定是否维护手写 Go SDK。  
不要让每个语言都手写一套协议。

### P3：GUI onboarding wizard

GUI 只做 CLI 的薄包装，不能成为唯一接入路径。  
在 CLI、doctor、Adapter 都稳定之前不做。

---

## 5. 为什么现在不做

```text
控制面 CLI 还没统一
配置面还没外部化
/meta 与 doctor 还没稳定
凭据文件格式还没定
Run 操作面还没闭环
```

这些是 Adapter 的地基。地基没稳，Adapter 会把临时接口固化。

---

## 6. 验收指标

```text
新 App 接入 ≤ 30 分钟
业务代码 ≤ 50 行
业务代码不出现 WebSocket / token / 重连 / 幂等
doctor / conformance 一次通过
同类接入问题的支持工单下降
```

---

## 7. 风险清单

```text
SDK 与协议漂移          → 用 conformance 用例锁住
多语言维护成本          → 先 Python + Sidecar，不铺开
Sidecar 安全边界        → 只监听 localhost，不持 Admin token
同一 app_id 多实例      → 先设计连接归属，再谈 Sidecar 横向扩展
Adapter 变成第二套服务端 → 明确 Adapter 只做传输与协议适配
```

---

## 8. 待确认

```text
[ ] 目标用户主要使用哪些语言？
[ ] Sidecar 的部署形态：同 Pod、同机、还是独立主机？
[ ] 接入方更需要嵌入式 SDK，还是本地 HTTP handler？
[ ] 多租户是否采用“一租户一 App”？
[ ] GUI wizard 是否真的需要，还是 CLI + doctor 足够？
```
