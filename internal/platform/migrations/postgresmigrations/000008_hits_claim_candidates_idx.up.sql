CREATE INDEX IF NOT EXISTS idx_hits_claim_candidates
    ON hits (message_date DESC, processing_until)
    WHERE category IS NULL
    OR llm_reason IS NULL
    OR BTRIM(llm_reason) = '';