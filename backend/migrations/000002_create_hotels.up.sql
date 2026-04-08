CREATE TABLE hotels (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name_en    TEXT NOT NULL,
    name_ru    TEXT,
    city       TEXT NOT NULL,
    address    TEXT,
    phone      TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_hotels_city ON hotels (city);
