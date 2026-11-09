-- Middleware — P5-42 App MCP references.
-- Copyright 2026 The Middleware Authors.

CREATE TABLE IF NOT EXISTS app_mcp_refs (
  app_id TEXT NOT NULL REFERENCES apps(app_id) ON DELETE CASCADE,
  mcp_id TEXT NOT NULL REFERENCES mcp_server_defs(mcp_id) ON DELETE CASCADE,
  PRIMARY KEY (app_id, mcp_id)
);

CREATE INDEX IF NOT EXISTS idx_app_mcp_refs_mcp
  ON app_mcp_refs(mcp_id);
