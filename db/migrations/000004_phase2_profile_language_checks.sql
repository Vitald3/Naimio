ALTER TABLE profile_languages
  ADD CONSTRAINT profile_languages_code_check
    CHECK (language_code = lower(language_code) AND language_code ~ '^[a-z]{2,3}(-[a-z0-9]{2,6})?$'),
  ADD CONSTRAINT profile_languages_level_check
    CHECK (level IN ('BASIC','CONVERSATIONAL','FLUENT','NATIVE'));
