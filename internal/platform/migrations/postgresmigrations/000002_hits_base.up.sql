CREATE TABLE IF NOT EXISTS hits (
                                    id           BIGSERIAL PRIMARY KEY,
                                    channel      TEXT NOT NULL,
                                    message_id   BIGINT NOT NULL,
                                    message_date TIMESTAMPTZ NOT NULL,
                                    text         TEXT NOT NULL,
                                    link         TEXT NOT NULL,
                                    keyword      TEXT NOT NULL,
                                    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    delivered_at TIMESTAMPTZ NULL,
    CONSTRAINT hits_channel_message_id_uq UNIQUE (channel, message_id)
    );