SELECT app_id, name, multiagent, status,
       length(api_key_hash) AS hash_len,
       api_key_hash ~ '^[0-9a-f]{64}$' AS hash_is_sha256_hex,
       api_key_hash LIKE 'mw_%' AS hash_is_plaintext,
       created_at, updated_at
FROM apps
WHERE app_id = 'app_01M729TVY98X39WFKR7S76JF4M';

SELECT count(*) AS rows_with_name
FROM apps
WHERE name = 'retrofit-p3-1794300014';
