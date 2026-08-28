DELETE FROM venue_cuisines;

DELETE FROM cuisines
WHERE slug IN (
    'bakery',
    'pizza',
    'sushi',
    'burgers',
    'italian',
    'japanese',
    'georgian',
    'asian',
    'desserts',
    'coffee'
);
