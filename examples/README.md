# examples

这里放可运行的参考 App 和客户端；**不负责**生产代码、协议定义或控制面实现。

```text
counter_agent/        推荐：Python SDK 薄 App（@tool + App）
http_client/          只验证 HTTP 的最小客户端
node_counter_agent/   Node.js 原生 WebSocket 参考 App
```

推荐入口：

```sh
levier up
levier model connect --url https://api.deepseek.com --key sk-... --model deepseek-chat
levier init --name counter-agent
levier dev
```

另开终端：

```sh
levier run "Add 2, then add 3, then read the counter."
```

从源码开发时安装 Python SDK：

```sh
python -m pip install -e ./sdk/python -e ./execution/runtime_sdk -e ./sdk/levier
```

参考实现只用于对照接入形态，不是协议规范。接入说明见
[`../docs/QUICKSTART.md`](../docs/QUICKSTART.md) 和
[`../sdk/levier/README.md`](../sdk/levier/README.md)。
