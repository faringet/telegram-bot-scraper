ALTER TABLE hits
    ADD COLUMN IF NOT EXISTS search_text TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS search_text_normalized TEXT NOT NULL DEFAULT '';

UPDATE hits
SET search_text = BTRIM(CONCAT_WS(' ', channel, keyword, text))
WHERE COALESCE(BTRIM(search_text), '') = '';

UPDATE hits
SET search_text_normalized = BTRIM(
        REGEXP_REPLACE(
                REGEXP_REPLACE(
                        LOWER(REPLACE(REPLACE(search_text, 'Ё', 'Е'), 'ё', 'е')),
                        '[^0-9a-zа-я\\s]+',
                        ' ',
                        'g'
                ),
                '\\s+',
                ' ',
                'g'
        )
                             )
WHERE COALESCE(BTRIM(search_text_normalized), '') = '';