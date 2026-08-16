CREATE TABLE proposals (
  id uuid PRIMARY KEY,
  project_id uuid NOT NULL REFERENCES projects(id),
  freelancer_user_id uuid NOT NULL REFERENCES users(id),
  message text NOT NULL CHECK (char_length(message) BETWEEN 1 AND 5000),
  price_kopecks bigint CHECK (price_kopecks IS NULL OR price_kopecks > 0),
  currency char(3) NOT NULL DEFAULT 'RUB' CHECK (currency = 'RUB'),
  delivery_days int CHECK (delivery_days IS NULL OR delivery_days BETWEEN 1 AND 3650),
  status text NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','SHORTLISTED','ACCEPTED','REJECTED','WITHDRAWN')),
  submitted_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  withdrawn_at timestamptz,
  UNIQUE(project_id, freelancer_user_id)
);
CREATE UNIQUE INDEX proposals_one_accepted_per_project ON proposals(project_id) WHERE status='ACCEPTED';
CREATE INDEX proposals_project_status_submitted_idx ON proposals(project_id,status,submitted_at DESC,id DESC);
CREATE INDEX proposals_freelancer_status_submitted_idx ON proposals(freelancer_user_id,status,submitted_at DESC,id DESC);

CREATE TABLE project_assignments (
  id uuid PRIMARY KEY,
  project_id uuid NOT NULL REFERENCES projects(id),
  proposal_id uuid REFERENCES proposals(id),
  freelancer_user_id uuid NOT NULL REFERENCES users(id),
  agreed_price_kopecks bigint CHECK (agreed_price_kopecks IS NULL OR agreed_price_kopecks > 0),
  currency char(3) NOT NULL DEFAULT 'RUB' CHECK (currency='RUB'),
  agreed_deadline_at timestamptz,
  status text NOT NULL DEFAULT 'ACTIVE' CHECK(status IN('ACTIVE','COMPLETED','CANCELLED')),
  started_at timestamptz NOT NULL,
  completed_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX assignments_one_active_per_project ON project_assignments(project_id) WHERE status='ACTIVE';
CREATE UNIQUE INDEX assignments_proposal_unique ON project_assignments(proposal_id) WHERE proposal_id IS NOT NULL;
