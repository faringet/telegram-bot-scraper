CREATE INDEX IF NOT EXISTS idx_hits_search_text_normalized_trgm
    ON hits
    USING gin (search_text_normalized gin_trgm_ops);