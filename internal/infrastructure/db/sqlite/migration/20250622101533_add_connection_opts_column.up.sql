ALTER TABLE settings
  ADD COLUMN ln_connection_datadir TEXT;
  
ALTER TABLE settings
  ADD COLUMN ln_connection_type INTEGER CHECK(ln_connection_type IN(0,1));

ALTER TABLE settings
  RENAME COLUMN ln_url TO ln_connection_url;