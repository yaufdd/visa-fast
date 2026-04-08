CREATE TABLE group_hotels (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id   UUID NOT NULL REFERENCES groups (id) ON DELETE CASCADE,
    hotel_id   UUID NOT NULL REFERENCES hotels (id),
    check_in   DATE NOT NULL,
    check_out  DATE NOT NULL,
    room_type  TEXT,
    sort_order INT  NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_group_hotels_group_id  ON group_hotels (group_id);
CREATE INDEX idx_group_hotels_hotel_id  ON group_hotels (hotel_id);
CREATE INDEX idx_group_hotels_sort      ON group_hotels (group_id, sort_order);
