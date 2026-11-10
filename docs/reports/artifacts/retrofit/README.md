<!-- Middleware — retrofit evidence index. -->

# Retrofit 证据目录

按 `docs/APP_RETROFIT_PLAN.md` 的步骤编号存放。

| 目录 | 内容 |
|---|---|
| `P0.1/` | 0 依赖 Counter Agent 基线输出 |
| `P0.2/` | 环境、迁移、健康、指标 |
| `P0.3/` | 真实 Harness Hello/在线证据 |
| `P0.4/` | 环境指纹（commit/版本/DB 目标，无密钥） |

## 规则

- 每个步骤一个目录。
- `summary.txt` 固定字段：step、commit、time、expected、observed、evidence。
- 不存放 API key、ws_token、Authorization、数据库密码。
- 数据库只记录 host/port/dbname 和 URL 指纹。
- 失败步骤保留失败输出，不覆盖为成功。
