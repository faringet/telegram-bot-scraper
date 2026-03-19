CREATE TABLE IF NOT EXISTS checkpoints (
                                           channel_username TEXT PRIMARY KEY,
                                           last_message_id  BIGINT NOT NULL,
                                           updated_at       TIMESTAMPTZ NOT NULL
);