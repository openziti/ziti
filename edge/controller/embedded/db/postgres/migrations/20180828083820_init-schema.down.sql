DO $$
BEGIN

  IF EXISTS(
      SELECT schema_name FROM information_schema.schemata WHERE schema_name = 'ziti_edge'
  )
  THEN
    EXECUTE 'DROP SCHEMA ziti_edge CASCADE';
  END IF;

END
$$;