# Levier — Python runtime image.
# Contains the Runtime SDK and both built-in runtimes.
# Copyright 2026 The Levier Authors.

FROM python:3.12-slim

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PYTHONPATH=/app/runtime_sdk/src:/app/react_runtime/src:/app/echo_runtime/src

WORKDIR /app

COPY execution/agent_core/pyproject.toml execution/agent_core/README.md /app/agent_core/
COPY execution/agent_core/src /app/agent_core/src
COPY execution/runtime_sdk/pyproject.toml execution/runtime_sdk/README.md /app/runtime_sdk/
COPY execution/runtime_sdk/src /app/runtime_sdk/src
COPY execution/echo_runtime/pyproject.toml execution/echo_runtime/README.md /app/echo_runtime/
COPY execution/echo_runtime/src /app/echo_runtime/src
COPY execution/echo_runtime/runtime.yaml /app/echo_runtime/runtime.yaml
COPY execution/react_runtime/pyproject.toml execution/react_runtime/README.md /app/react_runtime/
COPY execution/react_runtime/src /app/react_runtime/src
COPY execution/react_runtime/react.example.yaml /app/react_runtime/react.example.yaml

# Local workspace packages must be installed explicitly; pip does not read
# [tool.uv.sources].
RUN pip install --no-cache-dir --no-deps \
      -e /app/agent_core \
      -e /app/runtime_sdk \
      -e /app/echo_runtime \
      -e /app/react_runtime \
 && pip install --no-cache-dir \
      "grpcio>=1.75.0" "protobuf>=7.36.2" "pydantic>=2.14.3" "PyYAML>=6.0"

# Fail the image build if either runtime or the SDK import path is broken.
RUN PYTHONPATH=/app/runtime_sdk/src:/app/react_runtime/src \
      python -m levier_react_runtime --config /app/react_runtime/react.example.yaml --check \
 && PYTHONPATH=/app/runtime_sdk/src:/app/echo_runtime/src \
      python -m levier_echo_runtime --config /app/echo_runtime/runtime.yaml --check

ENTRYPOINT ["python", "-m", "levier_react_runtime"]
CMD ["--config", "/app/react_runtime/react.example.yaml"]
