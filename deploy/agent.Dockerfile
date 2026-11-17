# Levier — Counter Agent image.
# Copyright 2026 The Levier Authors.

FROM python:3.12-slim

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

WORKDIR /app
COPY examples/counter_agent /app/examples/counter_agent
COPY deploy/agent-entrypoint.sh /app/entrypoint.sh
RUN chmod 0755 /app/entrypoint.sh

ENTRYPOINT ["/app/entrypoint.sh"]
