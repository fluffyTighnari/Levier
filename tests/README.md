# tests

职责：跨单元测试、契约测试、E2E、检查脚本。

目录：

```text
contract/   协议黄金测试
e2e/        mvp-demo / retrofit / mcp / fault-matrix / i3-multireplica
app/        App 接入自检
checks/     依赖、结构、密钥、规模检查
```

入口：

```sh
make contract-check
make deps-check
make structure-check
make sdk-test
make mvp-demo
make mvp-demo-react
make fault-matrix
make i3-multireplica   # 2 control/api + 2 runtime-gateway + 2 app-gateway
make test-postgres     # Go integration tests; requires LEVIER_TEST_DATABASE_URL
make retrofit-e2e
make mcp-e2e
python3 tests/app/app_check.py --credentials /path/app.json
```

不负责：组件内部单元测试；它们与各自源码放在一起。

测试：本目录就是测试入口。
