-- Config sensitivity (plain|secret) and release sensitivity snapshot for redaction (S1; no encryption).

ALTER TABLE config_vars ADD COLUMN sensitivity TEXT NOT NULL DEFAULT 'plain';
ALTER TABLE shared_config_vars ADD COLUMN sensitivity TEXT NOT NULL DEFAULT 'plain';
ALTER TABLE releases ADD COLUMN config_sensitivity TEXT;
