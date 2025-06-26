ALTER TABLE settings
  DROP COLUMN ln_connection_datadir TEXT;
  
ALTER TABLE settings
  DROP COLUMN ln_connection_type INTEGER CHECK(ln_connection_type IN(0,1));

ALTER TABLE settings
  RENAME COLUMN ln_connection_url TO ln_url;