CREATE INDEX idx_config_versions_lookup
    ON config_versions (namespace, key, version DESC);