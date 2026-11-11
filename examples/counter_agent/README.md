<!-- Middleware — counter agent: baseline + Middleware runtime. -->

# Counter Agent（参考接入实现）

这个目录同时承载两件事：

```text
main.py    接入前的基线 App（进程内 ReAct，不连 Middleware）
serve.py   接入后的 App 运行时（Middleware 托管模型循环，App 只执行本地工具）
```

两者共用 `tools.py` / `dispatch.py` 的计数器语义，因此可以直接对照行为。

## 1. 两种模式

| | 基线 `main.py` | 接入 `serve.py` |
|---|---|---|
| 谁跑 ReAct 循环 | App 自己 | Middleware Harness |
| 谁调模型 | App 自己 | Harness（DeepSeek） |
| 工具在哪执行 | App 进程内 | App 进程内，经 WS 触发 |
| 是否需要 MW | 否 | 是 |
| 用途 | P0.1 基线、P6C 对照 | 真实接入 |

`main.py` 保持冻结是刻意的：P0.1 与 P6.9–P6.11 的行为对照依赖它。
接入没有“把 main 搬进 Middleware”，而是拆成两条职责：

```text
App：业务函数 + 工具元数据 + WebSocket 通道
MW ：注册表、调度、工具路由、幂等、事件
Harness：ReAct 循环 + 模型调用
```

## 2. 接入模式怎么跑

### 2.1 一次性 bootstrap

需要 MW 地址与 Admin token：

```sh
MW_URL=http://127.0.0.1:8080 \
MW_ADMIN_TOKEN=... \
MW_APP_NAME=counter-app \
python3 examples/counter_agent/bootstrap_app.py --credentials-out ~/.config/middleware/counter.json
```

产物是 0600 凭据文件：`app_id` / `app_name` / `api_key`。
key 只在这里出现一次，之后不再打印。

### 2.2 启动 App

```sh
python3 examples/counter_agent/serve.py \
  --credentials ~/.config/middleware/counter.json \
  --mw-url http://127.0.0.1:8080
```

`serve.py` 启动时依次做：

```text
load credentials
→ GET  /api/v1/apps/{app_id}      验证 key 与 active 状态
→ POST /api/v1/apps               全量注册 local tools
→ POST /api/v1/ws-token           换短期 token
→ GET  /api/v1/ws                 建立通道
→ 循环处理 local_tool_call / 回 local_tool_result
```

启动日志：

```json
{
  "serving": true,
  "app_id": "app_...",
  "app_name": "counter-app",
  "registered_tools": ["counter_add", "counter_get", "counter_reset"],
  "counter": 0
}
```

### 2.3 提交 Run

```sh
python3 examples/counter_agent/submit_run.py \
  --credentials ~/.config/middleware/counter.json \
  --mw-url http://127.0.0.1:8080 \
  --strategy react \
  --provider deepseek --model deepseek-flash \
  --prompt "Add 2, then add 3, then read the counter and tell me the final value."
```

Harness 跑模型循环，调用 `counter_add`/`counter_get` 时经 WS 回到 App，
App 执行后把结果返回，直到模型给出最终回答。

## 3. 真实运行结果

用基线同一条提示词跑接入模式（`serve.py`）：

```text
counter_add({"delta": 2}) -> {"value": 2}
counter_add({"delta": 3}) -> {"value": 5}
counter_get({})           -> {"value": 5}
run status: completed
```

与 `main.py` 基线的 final=5、3 次调用、add/add/get 顺序一致。
证据：`docs/reports/artifacts/onboarding/serve-e2e/`。

## 4. 目录职责（示例，不是接入规范）

| 文件 | 职责 |
|---|---|
| `tools.py` | 计数器状态与注册元数据 |
| `dispatch.py` | `tool_name + arguments_json → 结果/错误` |
| `credentials.py` | 0600 凭据读写 |
| `http_client.py` | App API（验证、注册、Run、ws-token） |
| `ws_client.py` | 标准库 WebSocket 帧与握手 |
| `bootstrap_app.py` | 一次性创建/轮换 App |
| `serve.py` | 运行时入口：验证 → 注册 → 连 WS → 服务 |
| `ws_tool_host.py` | 只连 WS 的诊断/探针（可 injected 初始值、计数退出） |
| `submit_run.py` | 提交 Run |
| `main.py` | 冻结的基线，不参与接入 |

接入其他 App 时不要求这个布局，见 `docs/APP_ONBOARDING.md`。

## 5. 安全

- App key 不写入源码、日志、证据、git。
- Admin token 只用于 bootstrap/rotate，不进入运行时。
- ws_token 只放内存。
- 工具结果按字符串透传，不做二次编码。

## 6. 测试

```sh
make counter-agent-test
```

覆盖：计数器语义、分发错误、凭据、HTTP 解析、WS 帧、工具回包、服务循环。

## 7. 相关文档

- `docs/APP_ONBOARDING.md`：接入流程（本项目版）
- `docs/APP_RETROFIT_PLAN.md`：逐边验证计划
- `docs/reports/REGISTRATION-CONTRACT.md`：注册合同实测
