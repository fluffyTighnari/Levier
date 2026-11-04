<!-- Middleware — protobuf contract workspace. -->

# Protobuf contracts

- `proto/middleware/runtime/v1/runtime.proto` — RuntimeChannel v1.
- `buf.yaml` — module, lint, and breaking-change policy.

Run:

```sh
buf lint
buf build
```

Generated Go/Python stubs are produced in P1-09; do not edit generated files by hand.
