<!-- Middleware — P8R.4 error and event consistency. -->

# P8R.4：错误与事件一致性

| 项目 | 内容 |
|---|---|
| 日期 | 2026-11-12 |
| 计划 | `docs/APP_RETROFIT_PLAN.md` P8R.4 |
| 状态 | 完成 |
| 代码提交 | 见下 |

## 1. 错误模型：两张表合成一张

之前 `internal/errs` 有两张平行 map：

```text
retryableByCode  map[Code]bool
httpStatusByCode map[Code]int
```

新增一个错误码时要同时改两处，容易漏。

现在合并为：

```go
type codeMeta struct {
    retryable bool
    status    int
}
var codeMetaByCode = map[Code]codeMeta{...}
```

`New()` 和 `HTTPStatus()` 都只读这一张表。

新增测试：

```text
TestCodeMetaIsComplete   每个 code 都有状态，HTTPStatus 与表一致
TestCodeMetaPins         固定关键码的 status/retryable
```

## 2. HTTP 错误输出：状态从 code 推导

新增：

```go
func writeErrorCode(w http.ResponseWriter, code errs.Code, message string, retryable bool)
```

所有 handler 改为：

```go
writeErrorCode(w, errs.CodeX, message, retryable)
```

不再手写 `http.StatusX + code` 组合。

结果：

```text
显式 status + code 的 writeError 调用：0
```

### 一个刻意修正

请求体过大之前是：

```text
413 Request Entity Too Large + BAD_REQUEST
```

现在统一为：

```text
413 + REQUEST_TOO_LARGE
```

`errs` 新增 `CodeRequestTooLarge`，测试同步更新。
这是行为变化，但消除了状态码与错误码的矛盾。

## 3. 事件字段常量

新增 `internal/contract/events.go`：

```text
EventFieldStatus
EventFieldInputTokens
EventFieldOutputTokens
```

`control/events.go` 不再手写这些 key：

```go
eventData[contract.EventFieldStatus]
data[contract.EventFieldInputTokens]
data[contract.EventFieldOutputTokens]
```

事件类型本身继续使用 proto 枚举，没有新增字符串常量。

跨语言的 payload schema 留给 P8R.5/P8R.6。

## 4. 验证

```text
go test ./...          全部通过
make lint              OK
make build             OK
dependency check       OK (9 packages with rules, 17 packages scanned)
size report            无文件 > 450 行
显式 status+code 组合   0
```

## 5. 证据

```text
docs/reports/artifacts/P8R.4/
  go_test.log
  lint.log
  build.log
  deps.txt
  size.txt
```

## 6. 下一步

P8R.5：App 接入套件。

```text
contracts/app-http.json
contracts/app-ws.json
scripts/app_check.py
APP_ONBOARDING 自检步骤
```
