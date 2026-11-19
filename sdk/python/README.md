# Levier Python App Adapter

Python App 只需要写业务工具；凭据、HTTP、WebSocket、重连、工具注册和
`local_tool_call` / `local_tool_result` 由本包处理。

## 最小示例

```python
from levier_app import LevierApp

app = LevierApp.from_credentials()


@app.tool(
    "counter_add",
    description="Add an integer to the counter.",
    schema={
        "type": "object",
        "properties": {"delta": {"type": "integer"}},
        "required": ["delta"],
        "additionalProperties": False,
    },
)
def counter_add(arguments):
    return {"value": arguments["delta"]}


if __name__ == "__main__":
    raise SystemExit(app.run())
```

本地安装：

```sh
pip install -e ./sdk/python
```

运行：

```sh
LEVIER_CREDENTIALS=~/.config/levier/counter-app.json \
  python app.py
```

工具 handler 是同步函数，接收已解析的 `arguments` 字典，返回值必须可 JSON
序列化。handler 抛出 `ToolExecutionError` 时可以指定 `code` 和 `retryable`；
其他异常会被转换为 `TOOL_EXEC_ERROR`。

公开 manifest 的机器可读定义在 `public/app-adapter/v1/schema.json`。

## 当前范围

```text
Python App adapter
标准库 HTTP / WebSocket
凭据文件复用 Levier CLI 的 canonical 格式
hello / ping / pong
工具注册和调用
断线重连
优雅停止
```

暂不包含：

```text
Node / TypeScript SDK
Sidecar
非 Python adapter
GUI
```

## 测试

```sh
PYTHONPATH=sdk/python/src python3 -m unittest discover -s sdk/python/tests -v
```
