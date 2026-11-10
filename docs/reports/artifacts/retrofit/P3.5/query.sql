SELECT tool_id, scope, app_id, name, description, schema_json, timeout_ms, version
FROM tool_defs
WHERE app_id = 'app_01M72BNV0VX8A1BHP7W6CN7KE6'
ORDER BY name;
