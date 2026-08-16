CREATE TABLE favorites (
  user_id uuid NOT NULL REFERENCES users(id),
  entity_type varchar(40) NOT NULL CHECK(entity_type IN('FREELANCER','SERVICE','PROJECT')),
  entity_id uuid NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY(user_id,entity_type,entity_id)
);
CREATE INDEX favorites_user_created_idx ON favorites(user_id,created_at DESC,entity_id DESC);
