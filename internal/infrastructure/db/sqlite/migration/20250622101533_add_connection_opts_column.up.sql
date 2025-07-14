ALTER TABLE settings ADD COLUMN ln_datadir TEXT;
ALTER TABLE settings ADD COLUMN ln_type INTEGER CHECK(ln_type IN(0,1, 2));