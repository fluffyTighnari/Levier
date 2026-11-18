# Levier public contracts

Files under `public/` are the contracts and examples that external clients may
depend on.

- `service-meta/v1/schema.json` — public schema for `GET /meta`
- `service-meta/v1/example.json` — non-secret example response
- `service-status/v1/schema.json` — public schema for `GET /status`
- `service-status/v1/example.json` — aggregate status example
- `service-selfcheck/v1/schema.json` — public schema for `levier selfcheck --json`
- `service-selfcheck/v1/example.json` — non-secret self-check example

Internal development documents live under `docs/` and are not shipped in the
container image, release archives, or HTTP responses.
