# examples

职责：参考 App 与最小客户端，用来对照接入方式。

目录：

```text
python_sdk_counter/   推荐：Python SDK 薄示例（业务 handler + schema）
counter_agent/        Python 标准库旧参考 App：bootstrap、serve、submit
http_client/          只验证 HTTP 的最小客户端
node_counter_agent/   Node.js 原生 WebSocket 参考 App
```

入口：

```sh
# 推荐的新 App 接入方式（从仓库根目录运行）
PYTHONPATH=sdk/python/src \
LEVIER_CREDENTIALS=~/.config/levier/sdk-counter.json \
  python3 examples/python_sdk_counter/app.py

# 旧参考实现仍保留，用于对照底层协议
python3 examples/counter_agent/bootstrap_app.py --credentials-out /path/to/credentials.json
python3 examples/counter_agent/serve.py --credentials /path/to/credentials.json
python3 examples/counter_agent/submit_run.py --credentials /path/to/credentials.json --strategy react
```

不负责：生产代码、协议定义、控制面实现。

测试：`make sdk-test`、`make counter-agent-test`、`examples/http_client/client.py`。

注意：新 App 首选 `docs/ONBOARDING.md` 和 `sdk/python/README.md`；旧示例仅用于对照底层协议。
