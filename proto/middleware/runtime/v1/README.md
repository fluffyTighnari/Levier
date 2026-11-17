<!-- Levier — RuntimeChannel v1 contract. -->

# RuntimeChannel v1

`runtime.proto` is the source of truth for the daemon ↔ middleware stream.

- One symmetric `Envelope` carries all messages.
- `Envelope.attempt` and `Envelope.lease_epoch` apply to nested messages.
- Persistent events consume `seq`; ephemeral events use `seq = 0`.
- MCP connections are described by `McpBinding`; middleware does not own them.

Validate with:

```sh
buf lint
buf build
```
