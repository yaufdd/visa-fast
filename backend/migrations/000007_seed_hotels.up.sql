-- Seed data: hotel database from FUJIT TRAVEL/CLAUDE.md
-- All hotels used for document generation must be kept in sync with that file.

INSERT INTO hotels (name_en, city, address, phone) VALUES

    -- Osaka
    ('Patina Osaka',
     'Osaka',
     '540-0007 Osaka, Chuo Ward, Banbacho, 391',
     '+81 6-6941-8888'),

    -- Kyoto
    ('Garrya Nijo Castle Kyoto',
     'Kyoto',
     'Ichinocho, Nakagyo Ward, Kyoto, 604-8305',
     '+81 75-366-5806'),

    ('Four Seasons Hotel Kyoto',
     'Kyoto',
     '445-3 Myohoin Maekawa-cho, Higashiyama-ku, Kyoto 605-0932',
     '+81 75-541-8288'),

    -- Hakone
    ('Gora Karaku',
     'Hakone',
     '1300-681 Gora, Hakone-machi, Kanagawa 250-0408',
     '+81 460-83-8860'),

    -- Izu
    ('Ochairo',
     'Izu',
     'Yuashima 1887-1',
     '+81 558-85-0014'),

    -- Okinawa
    ('The Busena Terrace',
     'Okinawa',
     '1808 Kise, Nago, Okinawa 905-0026',
     '+81 980-51-1333'),

    -- Tokyo
    ('Tokyo EDITION, Toranomon',
     'Tokyo',
     '4 Chome-1-1 Toranomon, Minato City, Tokyo',
     '+81 3-5422-1600'),

    ('Mesm Tokyo, Autograph Collection',
     'Tokyo',
     '2-7-2 Kaigan, Minato-ku, Tokyo 105-0022',
     '+81 3-5777-1111'),

    ('InterContinental ANA Tokyo by IHG',
     'Tokyo',
     '1-12-33 Akasaka, Minato-ku, Tokyo 107-0052',
     '+81 3-3505-1111');
