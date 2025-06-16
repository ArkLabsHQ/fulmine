CREATE TABLE IF NOT EXISTS subscribed_script (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    scripts TEXT NOT NULL
);