# Levier public contracts

Files under `public/` are the contracts and examples that external clients may
depend on.

- `service-meta/v1/schema.json` — public schema for `GET /meta`
- `service-meta/v1/example.json` — non-secret example response

Internal development documents live under `docs/` and are not shipped in the
container image, release archives, or HTTP responses.
