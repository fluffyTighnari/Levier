<!-- Middleware — RuntimeChannel contract compatibility rules. -->

# RuntimeChannel 契约冻结规则

| 项目 | 内容 |
|---|---|
| 版本 | v1 |
| 源文件 | `proto/middleware/runtime/v1/runtime.proto` |
| 生成物 | `gen/`、`execution/harness/src/gen/` |
| 基线 | `buf breaking --against '.git#branch=main'` |
| 黄金样本 | `tests/contract/envelopes.json` |

## 1. 唯一事实来源

- proto 是 RuntimeChannel 的唯一事实来源；`DESIGN.md` 与文档是镜像。
- Go/Python 生成代码只由 `buf generate` 产生，不手工修改。
- 字段语义变化必须先改 proto，再改文档/实现/黄金样本。

## 2. v1 允许的变更

1. 新增 message 字段，使用新的 field number。
2. 新增 enum value，保留 `*_UNSPECIFIED = 0`。
3. 新增 message 类型。
4. 新增 service method（需要明确的兼容策略）。

## 3. v1 禁止的变更

1. 删除字段或 message。
2. 重命名字段/消息/enum value。
3. 修改字段类型或 field number。
4. 复用已删除的 field number。
5. 改变 oneof 成员语义。
6. 把必填语义改成可选或反之，除非同时更新所有校验/文档/黄金样本。
7. 修改 `Envelope.attempt` / `Envelope.lease_epoch` 的作用域语义。

任何破坏性变更必须新建 `middleware.runtime.v2` 包，而不是改 v1。

## 4. 字段编号

- 新增字段使用下一个未使用编号；删除字段必须 `reserved` 编号和名称。
- oneof 内新增成员要确认所有 Handler 的 `default` 分支会拒绝未知 payload。
- `Envelope` 的 field 10–20 已分配；新增 payload 从 21 开始。

## 5. 事件与 fencing

- 持久化事件才消耗 `seq`；ephemeral 固定 `seq=0`。
- daemon 事件 `seq < 2^62`；MW 事件 `seq >= 2^62` 且 `source=mw`。
- 旧 daemon 由 `Envelope.lease_epoch` 识别；Run 重排队后必须递增。
- 事件的 `attempt` 来自 `Envelope.attempt`，用于 `(run_id, attempt, seq)` 分区。

## 6. JSON 表示

- 文档和黄金样本使用 proto 原字段名（snake_case）。
- 服务端 protojson 使用 `UseProtoNames: true`，避免生成代码默认 camelCase 与文档漂移。
- App HTTP API 使用 snake_case JSON；不要直接复用 protojson 的默认命名。
- proto3 JSON 的 int64 编码为字符串；黄金样本遵循该规则。

## 7. 检查

```sh
buf lint
buf build
buf breaking --against '.git#branch=main'
go test ./internal/protocol -run TestContractGolden
PYTHONPATH=execution/harness/src/gen:execution/harness/src python3 tests/contract/test_contract.py -v
```

黄金样本必须同时被 Go 和 Python 解析；任何一侧失败都视为契约破坏。
