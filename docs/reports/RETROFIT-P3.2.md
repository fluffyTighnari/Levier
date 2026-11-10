<!-- Middleware — retrofit P3.2 apps table persistence. -->

# Retrofit P3.2：确认 App 真实落库

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-10 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P3.2 |
| 状态 | 完成 |
| 代码提交 | 无（仅查询验证） |
| 基准提交 | `1326df1` |

## 1. 验证对象

P3.1 真实调用返回：

```text
app_id = app_01M729TVY98X39WFKR7S76JF4M
name   = retrofit-p3-1794300014
```

P3.2 直接查 PostgreSQL `apps` 表。

## 2. SQL

```sql
SELECT app_id, name, multiagent, status,
       length(api_key_hash) AS hash_len,
       api_key_hash ~ '^[0-9a-f]{64}$' AS hash_is_sha256_hex,
       api_key_hash LIKE 'mw_%' AS hash_is_plaintext,
       created_at, updated_at
FROM apps
WHERE app_id = 'app_01M729TVY98X39WFKR7S76JF4M';

SELECT count(*) AS rows_with_name
FROM apps
WHERE name = 'retrofit-p3-1794300014';
```

## 3. 实际结果

```text
app_id             = app_01M729TVY98X39WFKR7S76JF4M
name               = retrofit-p3-1794300014
multiagent         = false
status             = active
hash_len           = 64
hash_is_sha256_hex = true
hash_is_plaintext  = false
created_at         = 2026-11-10 16:40:14.537093+08
updated_at         = 2026-11-10 16:40:14.537093+08

rows_with_name     = 1
```

## 4. 这证明了什么

```text
P3.1 HTTP 201
  → handleAdminCreateApp
  → register.RegisterApp
  → INSERT INTO apps
  → PostgreSQL 行存在
```

同时证明：

- 响应中的 `app_id` 与 DB 行一致。
- `name` 与请求一致。
- App 状态为 `active`。
- `api_key_hash` 是 64 位 SHA-256 hex。
- 数据库没有 `mw_...` 明文 key。
- `name` 唯一，只有一行。

## 5. 没有证明什么

| 项目 | 说明 |
|---|---|
| App key 能认证 | P3.1 的 key 没有保存；P3.3 需要 rotate/重新获取 |
| 工具注册 | P3.4/P3.5 |
| `tool_defs` | P3.5 |
| WS | P5 |

## 6. 关于 API key

P3.1 的 `register_app.py` 故意只打印：

```text
has_api_key = true
```

没有保存明文。

结果是：

```text
DB 有 hash
App 侧没有 key
```

所以 P3.3 要解决：

```text
bootstrap 工具拿到 key
→ 安全交给 App runtime
→ App runtime 用 key 认证
```

可选方式：

1. bootstrap 后写 0600 secret file；
2. 写入 App runtime 环境；
3. 直接 rotate 后注入。

不能在日志/证据/源码中出现明文 key。

## 7. 证据

```text
docs/reports/artifacts/retrofit/P3.2/query.sql
docs/reports/artifacts/retrofit/P3.2/db_apps.txt
docs/reports/artifacts/retrofit/P3.2/summary.txt
```

## 8. 下一步

P3.3：App key 认证。

需要先解决 key 来源：

```text
用 admin token rotate 一个 key
→ 安全保存到 runtime
→ 调 GET /apps/{app_id}
→ 期望 200
→ 错误 key 期望 401
```
