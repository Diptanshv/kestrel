DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM information_schema.columns
    WHERE table_schema = 'public'
      AND table_name = 'sessions'
      AND column_name = 'token_has'
  ) THEN
    ALTER TABLE sessions RENAME COLUMN token_has TO token_hash;
  END IF;
END $$;
