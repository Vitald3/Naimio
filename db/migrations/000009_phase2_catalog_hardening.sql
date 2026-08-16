CREATE OR REPLACE FUNCTION enforce_category_depth() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE
  ancestor uuid;
  depth int := 1;
  subtree_depth int := 0;
BEGIN
  ancestor := NEW.parent_id;
  WHILE ancestor IS NOT NULL LOOP
    IF ancestor = NEW.id THEN
      RAISE EXCEPTION 'category cycle' USING ERRCODE = '23514';
    END IF;
    depth := depth + 1;
    IF depth > 3 THEN
      RAISE EXCEPTION 'category depth exceeds 3' USING ERRCODE = '23514';
    END IF;
    SELECT parent_id INTO ancestor FROM categories WHERE id = ancestor;
    IF NOT FOUND THEN
      RAISE EXCEPTION 'unknown parent category' USING ERRCODE = '23503';
    END IF;
  END LOOP;
  WITH RECURSIVE descendants(id, level) AS (
    SELECT id, 1 FROM categories WHERE parent_id = NEW.id
    UNION ALL
    SELECT c.id, d.level + 1 FROM categories c JOIN descendants d ON c.parent_id = d.id
    WHERE d.level < 3
  ) SELECT COALESCE(max(level), 0) INTO subtree_depth FROM descendants;
  IF depth + subtree_depth > 3 THEN
    RAISE EXCEPTION 'category subtree depth exceeds 3' USING ERRCODE = '23514';
  END IF;
  RETURN NEW;
END $$;

CREATE TRIGGER categories_depth_guard
BEFORE INSERT OR UPDATE OF parent_id ON categories
FOR EACH ROW EXECUTE FUNCTION enforce_category_depth();

CREATE INDEX IF NOT EXISTS categories_active_slug_idx ON categories(slug) WHERE is_active = true;
CREATE INDEX IF NOT EXISTS skills_active_name_idx ON skills(name) WHERE is_active = true;
