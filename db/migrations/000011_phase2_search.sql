CREATE INDEX IF NOT EXISTS users_display_name_trgm_idx ON users USING gin(display_name gin_trgm_ops) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS profiles_professional_title_trgm_idx ON professional_profiles USING gin(professional_title gin_trgm_ops);
CREATE INDEX IF NOT EXISTS profiles_bio_fts_idx ON professional_profiles USING gin(to_tsvector('simple',coalesce(professional_title,'')||' '||coalesce(bio,'')));
