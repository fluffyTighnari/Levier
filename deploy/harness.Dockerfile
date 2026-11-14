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

RUN pip install --no-cache-dir -e /app/agent_core \
 && pip install --no-cache-dir -e /app/harness

ENTRYPOINT ["python", "-m", "middleware_harness"]
CMD ["--config", "/app/harness/config.example.yaml"]
