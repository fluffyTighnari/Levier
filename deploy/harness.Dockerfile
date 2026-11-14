# Middleware — Python Harness image.
# Copyright 2026 The Middleware Authors.

FROM python:3.12-slim

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PYTHONPATH=/app/harness/src:/app/harness/src/gen

WORKDIR /app

COPY agent_core/pyproject.toml agent_core/README.md /app/agent_core/
COPY agent_core/src /app/agent_core/src
COPY harness/pyproject.toml harness/README.md /app/harness/
COPY harness/src /app/harness/src
COPY harness/config.example.yaml /app/harness/config.example.yaml

# middleware-agent-core is a local path dependency. Install it explicitly and
# keep pip from trying to resolve it from PyPI.
RUN pip install --no-cache-dir --no-deps -e /app/agent_core -e /app/harness \
 && pip install --no-cache-dir \
      "grpcio>=1.75.0" "protobuf>=7.36.2" "pydantic>=2.14.3" "PyYAML>=6.0"

# Fail the image build if generated protobuf modules or the adapter cannot import.
RUN PYTHONPATH=/app/harness/src:/app/harness/src/gen \
    python -m middleware_harness --config /app/harness/config.example.yaml --check

ENTRYPOINT ["python", "-m", "middleware_harness"]
CMD ["--config", "/app/harness/config.example.yaml"]
