---
title: 模型配置
status: published
description: 连接模型并验证：URL、API key、model 名称、adapter 与检查。
updated: 2026-11-21
---

## 连接

> [!NOTE]
> 模型是 React Runtime 的必需项；只跑 Echo 或自己的 Runtime 时可以先跳过。
> 本页讲 `model connect` 的参数与行为；验证见下文。

### 1. 最小配置

你只需要知道三件事：

```text
URL
API key
model 名称
```

```sh
levier model connect \
  --url https://api.deepseek.com \
  --key sk-... \
  --model deepseek-chat
```

> [!OUTPUT]
> ```text
> model connected: deepseek
> url:              https://api.deepseek.com
> model:            deepseek-chat
> default:          true
> next:             levier init
> ```

### 2. model connect 做了什么

```text
从 URL 推断 provider 名称
从 URL 推断 adapter
生成 secret_ref
保存 provider 元数据
把 key 写入控制面的 0600 managed secret 文件
设为默认 provider
立即检查 secret 是否可用
```

你不需要设置 `LEVIER_SECRET_*`，也不需要理解 `--secret-ref`。

### 3. 参数

| 参数 | 必填 | 说明 |
|---|---|---|
| `--url` | 是 | OpenAI-compatible 或 Anthropic 的 base URL |
| `--key` | 是 | API key；也可用 `LEVIER_MODEL_API_KEY` |
| `--model` | 是 | 默认模型名 |
| `--name` | 否 | 显式 provider 名称；URL 不明确时使用 |
| `--adapter` | 否 | `auto`（默认）、`openai_compatible`、`anthropic` |
| `--models` | 否 | 允许的模型列表，逗号分隔 |
| `--default` | 否 | 设为默认 provider；默认行为就是设为默认 |
| `--default=false` | 否 | 不改变默认 provider |
| `--server` | 否 | 控制面地址；默认读 profile |
| `--admin-token` | 否 | 管理 token；默认读 profile / 环境变量 |
| `--json` | 否 | 机器可读输出 |

支持的 adapter：

```text
openai_compatible   OpenAI 风格 /chat/completions
anthropic           Anthropic Messages API
```

### 4. 多模型与默认值

```sh
levier model connect \
  --name deepseek \
  --url https://api.deepseek.com \
  --key sk-... \
  --model deepseek-chat \
  --models deepseek-chat,deepseek-reasoner \
  --default
```

第二个 provider 可以不带 `--default`，避免覆盖默认值：

```sh
levier model connect \
  --name local \
  --url http://127.0.0.1:8000/v1 \
  --key local \
  --model qwen2.5 \
  --default=false
```

提交 Run 时可以显式选择：

```sh
levier run "hello" --provider deepseek --model deepseek-reasoner
```

### 5. 移除

```sh
levier model remove --name deepseek --yes
```

删除的是 provider 元数据；managed secret 文件里的值是否清理取决于控制面实现，
不要把它当作密钥擦除手段。轮换 key 请重新执行 `model connect`。

### 6. 常见失败

| 现象 | 先检查 |
|---|---|
| `401` / `403` | key 是否正确、是否过期 |
| `model not found` | `--model` 是否在 `--models` 中 |
| `connection refused` | base URL、代理、模型服务是否在线 |
| `SECRET_MISSING` | managed secret 文件是否可写、环境是否覆盖 |
| 连到了错误的 provider | 用 `--name` 和 `--adapter` 显式指定 |

## 验证

### 1. model list

```sh
levier model list
```

> [!OUTPUT]
> ```text
> name               model                default  base_url
> deepseek           deepseek-chat        true     https://api.deepseek.com
> ```

只显示 provider 元数据，不显示 `secret_ref`，也不显示 key。

JSON：

```sh
levier model list --json
```

### 2. model check

```sh
levier model check --name deepseek
```

> [!OUTPUT]
> ```text
> provider check: PASS deepseek
> ```

检查内容：

```text
provider 是否存在且 enabled
默认模型是否为空
secret 是否 configured
base_url 是否可解析
```

失败时输出 `provider check: FAIL`，并给出错误文本。

### 3. 常见失败

| 输出 / 现象 | 含义 | 动作 |
|---|---|---|
| `no models connected` | 还没有 provider | `levier model connect ...` |
| `provider not found` | `--name` 写错 | `levier model list` 确认名称 |
| `secret_configured false` | key 没有写入 | 重新 `model connect` |
| `401` / `403` | key 无效 | 换 key 后重新 connect |
| `model not found` | 默认模型不在允许列表 | 调整 `--models` 或 `--model` |
| `connection refused` | base URL 不可达 | 检查 URL、代理、模型服务 |

### 4. secret 状态

`model list` 不显示 secret_ref；需要看 secret 是否配置时：

```sh
levier model check --name deepseek
```

如果要看控制面侧的 provider 详情，使用管理员命令：

```sh
levier ops daemons
```

provider 的 secret_ref 不属于 App 可见面；App 只能通过
`App.list_model_providers()` 看到模型目录。

## 下一步

- MCP：[MCP 配置](pre-mcp.md)
- 密钥：[环境与密钥](pre-env.md)
- 完整接入：[完整接入](onboarding.md)
