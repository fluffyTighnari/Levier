# Levier — independent MCP runtime image.
# Copyright 2026 The Levier Authors.

FROM python:3.12-slim

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PYTHONPATH=/app/mcp_runtime/src

WORKDIR /app

COPY execution/mcp_runtime/pyproject.toml execution/mcp_runtime/README.md /app/mcp_runtime/
COPY execution/mcp_runtime/src /app/mcp_runtime/src

RUN pip install --no-cache-dir -e /app/mcp_runtime

ENTRYPOINT ["python", "-m", "levier_mcp_runtime"]
CMD ["--socket", "/tmp/levier-mcp.sock"]
