# examples

这里放可运行的参考 App 和客户端；**不负责**生产代码、协议定义或控制面实现。

```text
counter_agent/        推荐：Python SDK 薄 App（业务 handler + schema）
http_client/          只验证 HTTP 的最小客户端
node_counter_agent/   Node.js 原生 WebSocket 参考 App
```

推荐入口：

```sh
# 从仓库根目录运行
LEVIER_CREDENTIALS=./data/agent/credentials.json \
PYTHONPATH=sdk/python/src \
  python3 examples/counter_agent/app.py

LEVIER_CREDENTIALS=./data/agent/credentials.json \
  levier run submit \
  --strategy react \
  --prompt "Add 2, then add 3, then read the counter." \
  --watch
```

准备：

```sh
python3 -m venv .venv
.venv/Scripts/python -m pip install -e ./sdk/python

LEVIER_ADMIN_TOKEN=admin-change-me \
  levier app init \
  --name counter-agent \
  --server http://127.0.0.1:8080 \
  --credentials-out ./data/agent/credentials.json
```

参考实现只用于对照接入形态，不是协议规范。接入说明见
[`../sdk/python/README.md`](../sdk/python/README.md) 和
[`../docs/ONBOARDING.md`](../docs/ONBOARDING.md)。

测试：`make counter-agent-test`、`make sdk-test`、`examples/http_client/client.py`。
