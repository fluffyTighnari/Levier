<!-- Middleware — Node.js counter App (language independence). -->

# Node Counter App（P8）

一个最小接入实现，用来证明接入不依赖 Python、SDK 或生成代码。

## 依赖

```text
Node.js >= 22（使用内置全局 WebSocket）
没有 package.json
没有 node_modules
只 import node:http / node:https / node:fs
```

不需要：

```text
Middleware SDK
生成的 protobuf 代码
任何 npm 包
```

## 运行

先准备一个 0600 凭据文件（bootstrap 可由任意语言完成，这里是 Python）：

```sh
MW_URL=http://127.0.0.1:8080 \
MW_ADMIN_TOKEN=... \
MW_APP_NAME=node-counter \
python3 examples/counter_agent/bootstrap_app.py --credentials-out /tmp/node-app.json
```

启动 App：

```sh
node examples/node_counter_agent/serve.mjs \
  --credentials /tmp/node-app.json \
  --mw-url http://127.0.0.1:8080 \
  --calls 3 \
  --idle-seconds 60
```

它会在启动时：

```text
GET  /api/v1/apps/{app_id}     验证 key
POST /api/v1/apps              注册 counter_add/get/reset
POST /api/v1/ws-token          换 token
GET  /api/v1/ws                建立通道并 ping/pong
```

然后处理 `local_tool_call`：

```json
{"type":"local_tool_call","msg_id":"...","invocation_id":"...",
 "tool_name":"counter_add","arguments_json":"{\"delta\": 2}","timeout_ms":10000}
```

回包：

```json
{"type":"local_tool_result","correlation_id":"<msg_id>",
 "invocation_id":"...","ok":true,"result_json":"{\"value\":2}"}
```

## 真实结果

```text
ping → pong
counter_add {"delta": 2} -> {"value": 2}
counter_add {"delta": 3} -> {"value": 5}
counter_get {}           -> {"value": 5}
handled=3 counter=5
run completed
```

证据：`docs/reports/artifacts/retrofit/P8/`

## 说明

- `correlation_id` 必须回填收到的 `msg_id`。
- `arguments_json` / `result_json` 按字符串处理。
- 工具调用不要求订阅 Run 事件。
- 进程在 `--calls` 次调用后或空闲超时后退出。
