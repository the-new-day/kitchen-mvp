INSERT INTO cuisines (slug, name, position) VALUES
    ('bakery',    'Пекарня',        10),
    ('pizza',     'Пицца',          20),
    ('sushi',     'Суши',           30),
    ('burgers',   'Бургеры',        40),
    ('italian',   'Итальянская',    50),
    ('japanese',  'Японская',       60),
    ('georgian',  'Грузинская',     70),
    ('asian',     'Азиатская',      80),
    ('desserts',  'Десерты',        90),
    ('coffee',    'Кофе и напитки', 100)
ON CONFLICT (slug) DO NOTHING;
