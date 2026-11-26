<!-- Levier — RuntimeChannel cross-language golden fixtures. -->

# RuntimeChannel contract fixtures

`envelopes.json` contains canonical v1 envelopes in snake_case field form.
It is consumed by both:

- Go: `internal/contract/protocol/contract_test.go`
- Python: `tests/contract/test_contract.py`

The fixtures cover Hello, Heartbeat, TaskAssign, TaskAck, Event,
ToolCallRequest/Result, Cancel, Reconcile, Checkpoint, and ErrorReport.

Validation:

```sh
go test ./internal/contract/protocol -run TestContractGolden
PYTHONPATH=execution/runtime_sdk/src:execution/react_runtime/src python3 tests/contract/test_contract.py -v
```
