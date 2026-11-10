<!-- Middleware — Counter App retrofit target (P1). -->

# Counter App（改造目标）

这是 `examples/counter_agent/` 的改造目标工程。

当前只完成 **P1：本地 Counter 逻辑、工具分发、工具 manifest**。

## 结构

```text
examples/counter_app/
  src/counter.js      Counter 状态
  src/tools.js        local tool dispatcher
  src/manifests.js    注册/ TaskAssign 工具元数据
  test/               Node 内置 test runner
  DESIGN_MAPPING.md   代码 → DESIGN/proto 依据
```

## 依赖

- Node 24（内置 `node --test`）。
- 无 npm 依赖。
- P1 不连接 Middleware、不起 HTTP/WS。

## 运行测试

```sh
make retrofit-test
```

或：

```sh
cd examples/counter_app
node --test test/*.test.js
```

## 设计依据

每个模块都标注了 Middleware 设计来源，集中映射见：

- `DESIGN_MAPPING.md`
- `docs/APP_RETROFIT_PLAN.md` P1

## P1 不做什么

- 不注册 App / 工具。
- 不连接 WebSocket。
- 不调用 LLM。
- 不处理 `invocation_id` 幂等或超时。

这些分别属于 P3、P5、P6。
