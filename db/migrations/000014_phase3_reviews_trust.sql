CREATE TABLE reviews (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id uuid NOT NULL REFERENCES projects(id),
  reviewer_user_id uuid NOT NULL REFERENCES users(id),
  reviewee_user_id uuid NOT NULL REFERENCES users(id),
  reviewer_role text NOT NULL CHECK (reviewer_role IN ('CUSTOMER','FREELANCER')),
  rating_overall smallint NOT NULL CHECK (rating_overall BETWEEN 1 AND 5),
  would_work_again boolean,
  text text CHECK (text IS NULL OR char_length(text) <= 5000),
  status text NOT NULL DEFAULT 'PUBLISHED' CHECK (status IN ('PUBLISHED','HIDDEN')),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(project_id, reviewer_user_id),
  CHECK (reviewer_user_id <> reviewee_user_id)
);
CREATE INDEX reviews_reviewee_status_created_idx ON reviews(reviewee_user_id,status,created_at DESC,id DESC);
CREATE INDEX reviews_reviewer_created_idx ON reviews(reviewer_user_id,created_at DESC,id DESC);

CREATE TABLE review_dimensions (
  review_id uuid NOT NULL REFERENCES reviews(id) ON DELETE CASCADE,
  dimension varchar(80) NOT NULL,
  score smallint NOT NULL CHECK (score BETWEEN 1 AND 5),
  PRIMARY KEY(review_id, dimension)
);

CREATE TABLE user_trust_stats (
  user_id uuid PRIMARY KEY REFERENCES users(id),
  native_rating numeric(4,2),
  reviews_count int NOT NULL DEFAULT 0 CHECK (reviews_count >= 0),
  completed_projects_count int NOT NULL DEFAULT 0 CHECK (completed_projects_count >= 0),
  completion_rate numeric(5,2),
  on_time_rate numeric(5,2),
  repeat_rate numeric(5,2),
  recommendation_rate numeric(5,2),
  avg_response_minutes int,
  updated_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO user_trust_stats(user_id,completed_projects_count)
SELECT participant,count(DISTINCT project_id) FROM (
  SELECT p.customer_user_id AS participant,p.id AS project_id FROM projects p WHERE p.status='COMPLETED'
  UNION ALL
  SELECT a.freelancer_user_id,p.id FROM projects p JOIN project_assignments a ON a.project_id=p.id WHERE p.status='COMPLETED'
) completed GROUP BY participant
ON CONFLICT(user_id) DO UPDATE SET completed_projects_count=EXCLUDED.completed_projects_count,updated_at=now();
