# Ninja turtles

{
    "id": "storage.0",
    "pos": [50, 121, 822],
    "xlen": 4,
    "zlen": 4,
    "rows": 9
}

# Item export

1. Start a new creative world.
2. Open inventory > Options > Tools > Data dumps.
3. Go to the "Item panel" dump
4. Dump CSV and PNG (64x64).
5. Wait for dump to complete.
6. Copy /dumps/ to $web-root/items
7. Convert csv to JSON:
    ^([^,]+),(\d+),(\d+),(true|false),([^,]+)$
    "$1/$3": "$5",
8. Remove duplicates?
