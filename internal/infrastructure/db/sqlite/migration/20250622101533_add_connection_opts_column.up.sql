ALTER TABLE settings
  ADD COLUMN ln_connection_url TEXT;

ALTER TABLE settings
  ADD COLUMN ln_connection_datadir TEXT;

ALTER TABLE settings
  ADD COLUMN ln_connection_node INTEGER CHECK(ln_connection_node IN(0,1));