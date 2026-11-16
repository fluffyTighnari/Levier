# gen

职责：`proto/` 生成的 Go 代码存放处。

入口：

```text
gen/middleware/runtime/v1/runtime.pb.go
gen/middleware/runtime/v1/runtime_grpc.pb.go
```

生成方式：

```sh
make proto        # 需要 buf
# 或
buf generate
```

规则：

```text
不要手改本目录
契约源在 proto/
Python 生成物在 execution/harness/src/gen/
```

不负责：契约设计、业务逻辑、测试。

测试：`make proto-check`（生成物与源一致）。
