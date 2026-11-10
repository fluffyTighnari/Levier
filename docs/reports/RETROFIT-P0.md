<!-- Middleware — retrofit P0 baseline execution record. -->

# Retrofit P0：基线冻结与环境

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-10 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P0 |
| 状态 | 完成 |
| 结果 | 基线 final=5；MW/Harness 环境可用 |

## 1. P0.1 基线

命令：

```sh
make counter-agent
```

结果：

```text
[model] deepseek-flash
[tool] counter_add({"delta": 2}) -> {"value": 2}
[tool] counter_add({"delta": 3}) -> {"value": 5}
[tool] counter_get({}) -> {"value": 5}
[final] counter=5 steps=4
```

基线对照：

- final counter = 5
- tool calls = 3（add/add/get）
- 顺序 = add, add, get

证据：

- `docs/reports/artifacts/retrofit/P0.1/counter.log`
- `docs/reports/artifacts/retrofit/P0.1/summary.txt`

## 2. P0.2 环境

启动：

```sh
pg_ctl ... start
go run ./cmd/migrate -config config.example.yaml
bin/middleware -config config.example.yaml
```

结果：

```text
migrate=0
health={"status":"ok"}
/metrics 可读
```

证据：

- `P0.2/health.json`
- `P0.2/metrics.txt`
- `P0.2/migrate.log`
- `P0.2/middleware.log`

## 3. P0.3 真实 Harness

命令：

```sh
PYTHONPATH=harness/src/gen:harness/src \
  harness/.venv/bin/python -m middleware_harness \
  --config harness/config.example.yaml
```

结果：

```text
harness starting daemon_id=daemon-local-1
mw_daemon_capacity{daemon="daemon-local-1",kind="total",runtime="NORMAL"} 4
mw_daemon_capacity{daemon="daemon-local-1",kind="used",runtime="NORMAL"} 0
```

`mw_daemon_capacity` 只在 Hello 成功后出现，因此同时证明：

- gRPC 连接成功。
- daemon token 校验通过。
- daemon 注册进入 DaemonHub。

证据：

- `P0.3/harness.log`
- `P0.3/daemon_metrics.txt`
- `P0.3/daemon.txt`
- `P0.3/summary.txt`

## 4. P0.4 环境指纹

记录：

```text
commit=75b93bdca4cbb95df0ceeadaea2e91ddc4fb7c99
time_utc=2026-11-10T02:18:24Z
platform=macOS-15.5-arm64-arm-64bit
python3=3.9.6
harness_python=Python 3.14.7
node=v24.21.0
go=go version go1.26.0 darwin/arm64
db_target=postgres://127.0.0.1:5433/middleware
db_url_fingerprint=85c6fcdf592f
deepseek_api_key=present
```

未记录任何密钥。

证据：

- `P0.4/env.txt`

## 5. 结论

P0 通过：

- 基线可复现。
- PostgreSQL/MW/Harness 环境可用。
- 真实 Harness 已完成 Hello 并 online。
- 下一步可以从 P1.1 开始本地 Counter 逻辑改造。

## 6. 备注

- P0 不创建 App、不注册工具、不跑 Run。
- P0.3 使用真实 Harness，不使用 fakedaemon。
- P0 证据目录不含 API key、ws_token、DB 密码。
