# Middleware — Python Harness image.
# Copyright 2026 The Middleware Authors.

FROM python:3.12-slim

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PYTHONPATH=/app/harness/src:/app/harness/src/gen

WORKDIR /app

COPY execution/agent_core/pyproject.toml execution/agent_core/README.md /app/agent_core/
COPY execution/agent_core/src /app/agent_core/src
COPY execution/harness/pyproject.toml execution/harness/README.md /app/harness/
COPY execution/harness/src /app/harness/src
COPY execution/harness/levier.example.yaml /app/harness/levier.example.yaml

# levier-agent-core is a local path dependency. Install it explicitly and
# keep pip from trying to resolve it from PyPI.
RUN pip install --no-cache-dir --no-deps -e /app/agent_core -e /app/harness \
 && pip install --no-cache-dir \
      "grpcio>=1.75.0" "protobuf>=7.36.2" "pydantic>=2.14.3" "PyYAML>=6.0"

# Fail the image build if generated protobuf modules or the adapter cannot import.
RUN PYTHONPATH=/app/harness/src:/app/harness/src/gen \
    python -m levier_harness --config /app/harness/levier.example.yaml --check

ENTRYPOINT ["python", "-m", "levier_harness"]
CMD ["--config", "/app/harness/levier.example.yaml"]
