<!-- Middleware — retrofit evidence index. -->

# Retrofit 证据目录

按 `docs/APP_RETROFIT_PLAN.md` 的步骤编号存放。

## 目录

| 目录 | 步骤 | 内容 |
|---|---|---|
| `P0.1/` | P0.1 | 0 依赖 Counter Agent 基线输出 |
| `P0.2/` | P0.2 | 环境、迁移、健康、指标 |
| `P0.3/` | P0.3 | 真实 Harness Hello/在线 |
| `P0.4/` | P0.4 | 环境指纹（无密钥） |
| `P1.1/`–`P1.8/` | P1.1–P1.8 | Counter 工具抽取、分发、错误、注册元数据 |
| `P2.1/`–`P2.3/` | P2.1–P2.3 | 配置、bootstrap 请求、响应解析 |
| `P3.1/`–`P3.5/` | P3.1–P3.5 | App 创建、落库、认证、工具注册 |
| `REG-01/` | P0.5 | 注册合同 |
| `P4.1/` | P4.1–P4.3 | TaskAssign 工具可见、schema 一致、App 隔离 |
| `P4.4/` | P4.4 | schema 逐字节对比 |
| `P4.5/` | P4.5 | 两 App 工具隔离 |
| `P4.6/` | P4.6 | 删除工具后 TaskAssign 更新 |
| `P5.1/`–`P5.7/` | P5.1–P5.7 | ws_token、握手、ping/pong、订阅、续传、拒绝 |
| `P6A.1/`–`P6A.5/` | P6A.1–P6A.5 | LLM provider：JSON、SSE、错误、路由、凭据 |
| `P6.1/`–`P6.8/` | P6.1–P6.8 | react Run、tool_call、下发、回传、上下文、落库、事件、终态 |
| `P6.3f/` | P6.3f | 终态与 requeue 竞态修复 |
| `P6C/` | P6.9–P6.11 | 与 P0.1 基线对照 |
| `P6D/` | P6.12–P6.17 | 重复、correlation、未知、跨 App、断线、超时 |
| `P7/` | P7.1–P7.11 | 落库、trace、指标、脱敏、轮换、重启 |
| `P8/` | P8.1–P8.5 | Python HTTP + Node WS |
| `P9/` | P9.1–P9.9 | 断连重连、LLM 错误、daemon 丢失、MW 重启、依赖不可用 |
| `latency/` | 附加 | 工具往返与全链路延迟 |
| `P10/` | P10.1–P10.6 | 全量回归与一键脚本 |

## 规则

- 每个步骤一个目录；共享一次运行的步骤（P4.1–P4.3、P6.9–P6.11、P6.12–P6.17）合并存放并在 `summary.txt` 标注范围。
- `summary.txt` 固定字段：step、commit、time、expected、observed、evidence。
- 不存放 API key、ws_token、Authorization、数据库密码。
- 数据库只记录 host/port/dbname 和 URL 指纹。
- 失败步骤保留失败输出，不覆盖为成功。
- 一键脚本运行证据在 `P10/run-<UTC>/`，包含 `summary.txt`。
