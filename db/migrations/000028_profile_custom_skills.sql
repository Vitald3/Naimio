CREATE TABLE profile_custom_skills (
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name varchar(80) NOT NULL,
  sort_order smallint NOT NULL DEFAULT 0 CHECK (sort_order BETWEEN 0 AND 49),
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, name)
);
CREATE UNIQUE INDEX profile_custom_skills_user_name_ci_unique ON profile_custom_skills(user_id, lower(name));
