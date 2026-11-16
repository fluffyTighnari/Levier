# examples

职责：参考 App 与最小客户端，用来对照接入方式。

目录：

```text
counter_agent/        Python 标准库参考 App：bootstrap、serve、submit
http_client/          只验证 HTTP 的最小客户端
node_counter_agent/   Node.js 原生 WebSocket 参考 App
```

入口：

```sh
python3 examples/counter_agent/bootstrap_app.py --credentials-out /path/to/credentials.json
python3 examples/counter_agent/serve.py --credentials /path/to/credentials.json
python3 examples/counter_agent/submit_run.py --credentials /path/to/credentials.json --strategy react
```

不负责：生产代码、协议定义、控制面实现。

测试：`make counter-agent-test`、`examples/http_client/client.py`。

注意：示例不是接入规范；接入要求见 `docs/APP_ONBOARDING.md`。
