ALTER TABLE settings
  DROP COLUMN ln_datadir TEXT;
  
ALTER TABLE settings
  DROP COLUMN ln_type INTEGER CHECK(ln_type IN(0,1));
