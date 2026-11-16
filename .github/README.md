# .github

职责：GitHub Actions CI。

入口：`.github/workflows/ci.yml`。

当前 job：

```text
go      make lint / test / build
proto   make proto-lint / proto-check
python  harness / contract 测试
```

不负责：本地开发脚本、部署、运行时配置。

本地对应：

```sh
make lint
make test
make build
make proto-lint
make proto-check
make harness-test
```
