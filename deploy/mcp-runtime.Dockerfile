# Middleware — independent MCP runtime image.
# Copyright 2026 The Middleware Authors.

FROM python:3.12-slim

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PYTHONPATH=/app/mcp_runtime/src

WORKDIR /app

COPY mcp_runtime/pyproject.toml mcp_runtime/README.md /app/mcp_runtime/
COPY mcp_runtime/src /app/mcp_runtime/src

RUN pip install --no-cache-dir -e /app/mcp_runtime

ENTRYPOINT ["python", "-m", "mcp_runtime"]
CMD ["--socket", "/tmp/middleware-mcp.sock"]
