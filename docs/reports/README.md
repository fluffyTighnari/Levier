<!-- Middleware — implementation report index. -->

# 实施报告索引

从 **P0-05** 起，每个步骤完成后必须新增一份 `docs/reports/Px-yy.md`，记录该步实际新增内容、验证结果、风险与下一步。

| 步骤 | 报告 | 状态 |
|---|---|---|
| P0-01 | `docs/ARCHIVE.md` / ADR-0001 前的归档记录 | 历史，未按本格式 |
| P0-02 | `docs/adr/0001-workflow-engine.md` | 历史，未按本格式 |
| P0-03 | `docs/adr/0002-harness.md` | 历史，未按本格式 |
| P0-04 | `docs/adr/0003-scope.md` | 历史，未按本格式 |
| P0-05 | `docs/reports/P0-05.md` | 已提交 |
| P0-06 | `docs/reports/P0-06.md` | 已提交 |
| P0-07 | `docs/reports/P0-07.md` | 已提交 |
| P1-08 | `docs/reports/P1-08.md` | 已提交 |
| P1-09 | `docs/reports/P1-09.md` | 已提交 |
| P1-10 | `docs/reports/P1-10.md` | 已提交 |
| P1-11 | `docs/reports/P1-11.md` | 已提交 |
| P1-12 | `docs/reports/P1-12.md` | 已提交 |
| P1-13 | `docs/reports/P1-13.md` | 已提交（G1 待确认） |
| P2-14 | `docs/reports/P2-14.md` | 已提交 |
| P2-15 | `docs/reports/P2-15.md` | 已提交 |
| P2-16 | `docs/reports/P2-16.md` | 已提交 |
| P2-17 | `docs/reports/P2-17.md` | 已提交 |
| P2-18 | `docs/reports/P2-18.md` | 已提交 |
| P2-19 | `docs/reports/P2-19.md` | 已提交 |
| P2-20 | `docs/reports/P2-20.md` | 已提交 |
| P2-21 | `docs/reports/P2-21.md` | 已提交 |
| P2-22 | `docs/reports/P2-22.md` | 已提交（G2 通过） |
| P3-23 | `docs/reports/P3-23.md` | 已提交 |
| P3-24 | `docs/reports/P3-24.md` | 已提交 |
| P3-25 | `docs/reports/P3-25.md` | 已提交 |
| P3-26 | `docs/reports/P3-26.md` | 已提交 |
| P3-27 | `docs/reports/P3-27.md` | 已提交 |
| P3-28 | `docs/reports/P3-28.md` | 已提交 |
| P3-29 | `docs/reports/P3-29.md` | 已提交 |
| P3-30 | `docs/reports/P3-30.md` | 已提交 |
| G3 | `docs/reports/G3.md` | 已提交 |
| P4-31 | `docs/reports/P4-31.md` | 已提交 |
| P4-32 | `docs/reports/P4-32.md` | 已提交 |
| P4-33 | `docs/reports/P4-33.md` | 已提交 |
| P4-34 | `docs/reports/P4-34.md` | 已提交 |
| P4-35 | `docs/reports/P4-35.md` | 已提交 |
| P4-36 | `docs/reports/P4-36.md` | 已提交 |
| P5-37 | `docs/reports/P5-37.md` | 已提交 |
| P5-38 | `docs/reports/P5-38.md` | 已提交 |
| P5-39 | `docs/reports/P5-39.md` | 已提交 |
| P5-40 | `docs/reports/P5-40.md` | 已提交 |
| P5-41 | `docs/reports/P5-41.md` | 已提交 |
| P5-42 | `docs/reports/P5-42.md` | 已提交 |
| G5 | `docs/reports/G5.md` | 已提交 |
| P6-43 | `docs/reports/P6-43.md` | 已提交 |
| P6-44 | `docs/reports/P6-44.md` | 已提交 |
| P6-45 | `docs/reports/P6-45.md` | 已提交 |
| P6-46 | `docs/reports/P6-46.md` | 已提交 |
| P6-47 | `docs/reports/P6-47.md` | 已提交 |
| P6-48 | `docs/reports/P6-48.md` | 已提交 |
| P6-49 | `docs/reports/P6-49.md` | 已提交 |
| P6-50 | `docs/reports/P6-50.md` | 已提交 |
| P6-51 | `docs/reports/P6-51.md` | 已提交 |
| G6 | `docs/reports/G6.md` | 已提交 |
| P7-52 | `docs/reports/P7-52.md` | 已提交 |
| P7-53 | `docs/reports/P7-53.md` | 已提交 |
| P7-54 | `docs/reports/P7-54.md` | 已提交 |
| P7-55 | `docs/reports/P7-55.md` | 已提交 |
| P7-56 | `docs/reports/P7-56.md` | 已提交 |
| P7-57 | `docs/reports/P7-57.md` | 已提交 |
| P7-58 | `docs/reports/P7-58.md` | 已提交 |
| G7 | `docs/reports/G7.md` | 已提交 |
| P8-62 | `docs/reports/P8-62.md` | 观测基线完成 |
| P8-63 | `docs/reports/P8-63.md` | 安全基线完成 |
| G4 | `docs/reports/G4.md` | 已提交 |

模板：`docs/reports/TEMPLATE.md`。

## 专题报告

| 报告 | 内容 |
|---|---|
| `docs/reports/WORKFLOW_TRACE.md` | Workflow 层全量逐函数追踪、输入输出、竞态与测试说明 |
| `docs/reports/BOUNDARY_DATA_FLOW.md` | Middleware/Daemon 数据结构与边界流：Envelope、Task、事件、工具、重连 |
| `docs/reports/APP-BASELINE.md` | 0 依赖 Counter Agent 基线与语言无关 HTTP 验证 |
| `docs/reports/RETROFIT-P0.md` | App 改造 P0：基线冻结与环境验证 |
| `docs/reports/REGISTRATION-CONTRACT.md` | App/local tool 注册格式与落库/TaskAssign 实测 |
| `docs/reports/RETROFIT-P1.1.md` | App 改造 P1.1：Counter 函数与注册元数据抽取 |
| `docs/reports/RETROFIT-P1.2.md` | App 改造 P1.2：dispatcher counter_add |
| `docs/reports/RETROFIT-P1.3-P1.4.md` | App 改造 P1.3–P1.4：dispatcher get/reset |
| `docs/reports/RETROFIT-P1.5-P1.6.md` | App 改造 P1.5–P1.6：dispatcher 异常处理 |
| `docs/reports/RETROFIT-P1.7.md` | App 改造 P1.7：注册 schema 与 TaskAssign manifest |
| `docs/reports/RETROFIT-P1.8.md` | App 改造 P1.8：main 接 dispatcher，P1 收口 |
| `docs/reports/RETROFIT-P2.1.md` | App 改造 P2.1：HTTP 配置读取 |
| `docs/reports/RETROFIT-P2.2.md` | App 改造 P2.2：构造 `/admin/apps` 请求 |
| `docs/reports/RETROFIT-P2.3.md` | App 改造 P2.3：解析 Admin App 响应（mock guardrail） |
| `docs/reports/RETROFIT-P3.1.md` | App 改造 P3.1：真实调用 `POST /admin/apps` |
| `docs/reports/RETROFIT-P3.2.md` | App 改造 P3.2：确认 App 真实落库 |
| `docs/reports/RETROFIT-P3.3.md` | App 改造 P3.3：App key 认证与可复用 bootstrap |
| `docs/reports/RETROFIT-P3.4.md` | App 改造 P3.4：真实注册 local tools |
| `docs/reports/RETROFIT-P3.5.md` | App 改造 P3.5：确认 tool_defs 真实落库 |
| `docs/reports/RETROFIT-P4.1-P4.3.md` | App 改造 P4.1–P4.3：TaskAssign 工具可见 |
| `docs/reports/RETROFIT-P4.4.md` | App 改造 P4.4：TaskAssign schema 精确对比 |
| `docs/reports/RETROFIT-P4.5.md` | App 改造 P4.5：App 间工具隔离 |
| `docs/reports/RETROFIT-P4.6.md` | App 改造 P4.6：删除工具后 TaskAssign 更新 |
| `docs/reports/RETROFIT-P5.1.md` | App 改造 P5.1：App key 换 ws_token |
| `docs/reports/RETROFIT-P5.2.md` | App 改造 P5.2：真实 WebSocket 连接 |
| `docs/reports/RETROFIT-P5.3.md` | App 改造 P5.3：真实 WebSocket ping/pong |
| `docs/reports/RETROFIT-P5.4.md` | App 改造 P5.4：subscribe 空 Run 与 caught_up |
| `docs/reports/RETROFIT-P5.5.md` | App 改造 P5.5：subscribe 后接收真实事件 |
| `docs/reports/RETROFIT-P5.6.md` | App 改造 P5.6：断线重连与 cursor |
| `docs/reports/RETROFIT-P5.7.md` | App 改造 P5.7：ws_token 过期与跨 App 拒绝 |
| `docs/reports/RETROFIT-P6A.1.md` | App 改造 P6A.1：OpenAI-compatible JSON 解析 |
| `docs/reports/RETROFIT-P6A.2.md` | App 改造 P6A.2：SSE 流式解析 |
| `docs/reports/RETROFIT-P6A.3.md` | App 改造 P6A.3：OpenAI-compatible 错误映射 |
| `docs/reports/RETROFIT-P6A.4.md` | App 改造 P6A.4：Provider 路由 |
| `docs/reports/RETROFIT-P6A.5.md` | App 改造 P6A.5：DeepSeek 凭据与真实 smoke |
| `docs/reports/RETROFIT-P6.1.md` | App 改造 P6.1：真实 react Run + TaskAssign tools |
| `docs/reports/RETROFIT-P6.2.md` | App 改造 P6.2：DeepSeek tool_call → ToolCallRequest |
| `docs/reports/RETROFIT-P6.3.md` | App 改造 P6.3：App 收到 local_tool_call |
| `docs/reports/RETROFIT-P6.3f.md` | App 改造 P6.3f：终态与 requeue 竞态 |
| `docs/reports/RETROFIT-P6.4.md` | App 改造 P6.4：App 返回 local_tool_result |
| `docs/reports/RETROFIT-P6.5.md` | App 改造 P6.5：ToolCallResult 进入下一轮模型输入 |
| `docs/reports/RETROFIT-P6.6.md` | App 改造 P6.6：tool_invocations 落库 |
| `docs/reports/RETROFIT-P6.7.md` | App 改造 P6.7：工具事件成对与字段完整 |
| `docs/reports/RETROFIT-P6.8.md` | App 改造 P6.8：Run 终态与最终回答 |
| `docs/reports/RETROFIT-LATENCY.md` | App 改造：P6B 链路延迟测量 |
| `docs/reports/RETROFIT-P6C.md` | App 改造 P6C：与基线对照（P6.9–P6.11） |
| `docs/reports/RETROFIT-P6D.md` | App 改造 P6D：负向与故障（P6.12–P6.17） |
| `docs/reports/RETROFIT-P7.md` | App 改造 P7：持久化与观测（P7.1–P7.11） |
| `docs/reports/RETROFIT-P8.md` | App 改造 P8：语言无关（P8.1–P8.5） |
| `docs/reports/RETROFIT-P9.md` | App 改造 P9：故障与恢复 |
| `docs/reports/RETROFIT-E2E.md` | App 改造 P10：回归与一键收口 |
| `docs/APP_ONBOARDING.md` | 任意 App 接入 Middleware 的标准流程 |

**规则：**

1. 报告在代码/文档提交的同一轮完成，不能事后补写。
2. 报告必须列出实际新增/修改文件，不能只写“实现完成”。
3. 报告必须包含可复现的验证命令与结果。
4. 报告必须记录未完成项与风险；没有风险就写“无”，不能省略。
5. 下一步开始前，先确认上一份报告存在且已提交。
