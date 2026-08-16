ALTER TABLE projects DROP CONSTRAINT projects_status_check;
ALTER TABLE projects ADD CONSTRAINT projects_status_check CHECK(status IN('DRAFT','OPEN','MATCHING','AWAITING_FUNDING','IN_PROGRESS','COMPLETED','CANCELLED','ARCHIVED'));
DO $$ DECLARE n text; BEGIN SELECT conname INTO n FROM pg_constraint WHERE conrelid='projects'::regclass AND contype='c' AND pg_get_constraintdef(oid) LIKE '%published_at%'; IF n IS NOT NULL THEN EXECUTE format('ALTER TABLE projects DROP CONSTRAINT %I',n); END IF; END $$;
ALTER TABLE projects ADD CONSTRAINT projects_published_state_check CHECK(status NOT IN('OPEN','MATCHING','AWAITING_FUNDING','IN_PROGRESS','COMPLETED') OR published_at IS NOT NULL);
ALTER TABLE projects DROP CONSTRAINT projects_source_type_check;
ALTER TABLE projects ADD CONSTRAINT projects_source_type_check CHECK(source_type IN('MANUAL','AI_BRIEF','IMPORT','COMMERCIAL_OFFER','CALCULATOR','REPEAT','INVITE'));
ALTER TABLE project_assignments ALTER COLUMN started_at DROP NOT NULL;

CREATE TABLE safe_deal_fee_rules(
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), version int NOT NULL UNIQUE CHECK(version>0),
  commission_basis_points int NOT NULL CHECK(commission_basis_points BETWEEN 0 AND 10000),
  minimum_fee_kopecks bigint NOT NULL DEFAULT 0 CHECK(minimum_fee_kopecks>=0),
  maximum_fee_kopecks bigint CHECK(maximum_fee_kopecks IS NULL OR maximum_fee_kopecks>=minimum_fee_kopecks),
  enabled boolean NOT NULL DEFAULT false, effective_from timestamptz NOT NULL DEFAULT now(), created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX safe_deal_fee_rules_enabled_unique ON safe_deal_fee_rules(enabled) WHERE enabled;
INSERT INTO safe_deal_fee_rules(version,commission_basis_points,minimum_fee_kopecks,enabled) VALUES(1,1000,0,true);

CREATE TABLE safe_deals(
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), project_id uuid NOT NULL REFERENCES projects(id), assignment_id uuid NOT NULL REFERENCES project_assignments(id),
  customer_user_id uuid NOT NULL REFERENCES users(id), freelancer_user_id uuid NOT NULL REFERENCES users(id),
  currency char(3) NOT NULL DEFAULT 'RUB' CHECK(currency='RUB'), gross_amount_kopecks bigint NOT NULL CHECK(gross_amount_kopecks>0),
  platform_fee_kopecks bigint NOT NULL CHECK(platform_fee_kopecks>=0 AND platform_fee_kopecks<gross_amount_kopecks),
  freelancer_amount_kopecks bigint NOT NULL CHECK(freelancer_amount_kopecks=gross_amount_kopecks-platform_fee_kopecks AND freelancer_amount_kopecks>0),
  fee_rule_version int NOT NULL, status text NOT NULL CHECK(status IN('AWAITING_FUNDING','FUNDED','IN_PROGRESS','SUBMITTED','REVISION_REQUESTED','DISPUTED','ACCEPTED','RELEASE_PENDING','COMPLETED','CANCEL_PENDING','CANCELED','REFUND_PENDING','REFUNDED','FAILED')),
  proposal_snapshot jsonb NOT NULL CHECK(jsonb_typeof(proposal_snapshot)='object'), project_snapshot jsonb NOT NULL CHECK(jsonb_typeof(project_snapshot)='object'),
  revision_count int NOT NULL DEFAULT 0 CHECK(revision_count BETWEEN 0 AND 20), auto_accept_at timestamptz,
  funded_at timestamptz, work_started_at timestamptz, submitted_at timestamptz, accepted_at timestamptz, completed_at timestamptz, canceled_at timestamptz,
  version int NOT NULL DEFAULT 1 CHECK(version>0), created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
  CHECK(customer_user_id<>freelancer_user_id)
);
CREATE UNIQUE INDEX safe_deals_assignment_unique ON safe_deals(assignment_id);
CREATE UNIQUE INDEX safe_deals_active_project_unique ON safe_deals(project_id) WHERE status NOT IN('COMPLETED','CANCELED','REFUNDED','FAILED');
CREATE INDEX safe_deals_customer_updated_idx ON safe_deals(customer_user_id,updated_at DESC,id DESC);
CREATE INDEX safe_deals_freelancer_updated_idx ON safe_deals(freelancer_user_id,updated_at DESC,id DESC);

CREATE TABLE safe_deal_events(
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), deal_id uuid NOT NULL REFERENCES safe_deals(id), event_type varchar(120) NOT NULL,
  actor_user_id uuid REFERENCES users(id), operation_key varchar(200) NOT NULL, metadata jsonb NOT NULL DEFAULT '{}' CHECK(jsonb_typeof(metadata)='object'), created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(deal_id,operation_key)
);
CREATE INDEX safe_deal_events_deal_created_idx ON safe_deal_events(deal_id,created_at,id);

CREATE TABLE payment_records(
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), deal_id uuid NOT NULL REFERENCES safe_deals(id), provider varchar(80) NOT NULL,
  provider_payment_id varchar(200), provider_status varchar(80) NOT NULL, amount_kopecks bigint NOT NULL CHECK(amount_kopecks>0), currency char(3) NOT NULL CHECK(currency='RUB'),
  idempotency_key varchar(160) NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(deal_id,idempotency_key)
);
CREATE UNIQUE INDEX payment_records_provider_payment_unique ON payment_records(provider,provider_payment_id) WHERE provider_payment_id IS NOT NULL;
CREATE INDEX payment_records_unresolved_idx ON payment_records(updated_at) WHERE provider_status IN('PENDING','FUNDED','RELEASE_PENDING','REFUND_PENDING','CANCEL_PENDING');

CREATE TABLE payment_events(
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), provider varchar(80) NOT NULL, provider_event_id varchar(200) NOT NULL,
  payment_record_id uuid REFERENCES payment_records(id), event_type varchar(120) NOT NULL, verified boolean NOT NULL,
  payload jsonb NOT NULL CHECK(jsonb_typeof(payload)='object'), received_at timestamptz NOT NULL DEFAULT now(), processed_at timestamptz, error_code varchar(100),
  UNIQUE(provider,provider_event_id)
);

CREATE TABLE safe_deal_submissions(
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), deal_id uuid NOT NULL REFERENCES safe_deals(id), revision_number int NOT NULL CHECK(revision_number>=0),
  submitted_by_user_id uuid NOT NULL REFERENCES users(id), summary text NOT NULL CHECK(char_length(summary) BETWEEN 1 AND 5000),
  message_id uuid REFERENCES messages(id), created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(deal_id,revision_number)
);

CREATE TABLE safe_deal_disputes(
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), deal_id uuid NOT NULL REFERENCES safe_deals(id), opened_by_user_id uuid NOT NULL REFERENCES users(id),
  reason_code varchar(80) NOT NULL CHECK(reason_code IN('WORK_NOT_DELIVERED','WORK_DOES_NOT_MATCH_SCOPE','DEADLINE_MISSED','CUSTOMER_UNRESPONSIVE','UNREASONABLE_REVISION','FRAUD_SUSPECTED','MUTUAL_CANCELLATION','OTHER')),
  description text NOT NULL CHECK(char_length(description) BETWEEN 3 AND 5000), status text NOT NULL DEFAULT 'OPEN' CHECK(status IN('OPEN','EVIDENCE_COLLECTION','UNDER_REVIEW','RESOLVED_CUSTOMER','RESOLVED_FREELANCER','CANCELED')),
  resolution varchar(40), resolution_reason text, opened_at timestamptz NOT NULL DEFAULT now(), resolved_at timestamptz, resolved_by_admin_id uuid REFERENCES users(id)
);
CREATE UNIQUE INDEX safe_deal_disputes_open_unique ON safe_deal_disputes(deal_id) WHERE status IN('OPEN','EVIDENCE_COLLECTION','UNDER_REVIEW');
CREATE INDEX safe_deal_disputes_queue_idx ON safe_deal_disputes(status,opened_at);

CREATE TABLE safe_deal_dispute_evidence(
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), dispute_id uuid NOT NULL REFERENCES safe_deal_disputes(id) ON DELETE CASCADE,
  author_user_id uuid NOT NULL REFERENCES users(id), kind varchar(30) NOT NULL CHECK(kind IN('COMMENT','MESSAGE_REFERENCE','SUBMISSION_REFERENCE')),
  body text NOT NULL CHECK(char_length(body) BETWEEN 1 AND 5000), reference_id uuid, created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE safe_deal_command_results(
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), deal_id uuid NOT NULL REFERENCES safe_deals(id), actor_user_id uuid REFERENCES users(id),
  action varchar(80) NOT NULL, idempotency_key varchar(160) NOT NULL, request_hash bytea NOT NULL, response jsonb NOT NULL DEFAULT '{}', created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(deal_id,action,idempotency_key)
);

CREATE OR REPLACE FUNCTION create_safe_deal_for_assignment() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE fee_rule safe_deal_fee_rules%ROWTYPE; fee bigint; customer uuid; proposal jsonb; project jsonb;
BEGIN
  IF NEW.agreed_price_kopecks IS NULL OR NEW.agreed_price_kopecks<=0 THEN RAISE EXCEPTION 'accepted proposal requires agreed price' USING ERRCODE='23514'; END IF;
  SELECT * INTO fee_rule FROM safe_deal_fee_rules WHERE enabled AND effective_from<=now() ORDER BY effective_from DESC LIMIT 1;
  IF NOT FOUND THEN RAISE EXCEPTION 'safe deal fee rule missing'; END IF;
  fee:=greatest(fee_rule.minimum_fee_kopecks,(NEW.agreed_price_kopecks*fee_rule.commission_basis_points)/10000);
  IF fee_rule.maximum_fee_kopecks IS NOT NULL THEN fee:=least(fee,fee_rule.maximum_fee_kopecks); END IF;
  IF fee>=NEW.agreed_price_kopecks THEN RAISE EXCEPTION 'safe deal fee must be lower than gross amount' USING ERRCODE='23514'; END IF;
  SELECT p.customer_user_id,jsonb_build_object('title',p.title,'description',p.description,'deadline_at',p.deadline_at,'budget_type',p.budget_type),
         jsonb_build_object('proposal_id',pr.id,'message',pr.message,'price_kopecks',pr.price_kopecks,'currency',pr.currency,'delivery_days',pr.delivery_days)
    INTO customer,project,proposal FROM projects p JOIN proposals pr ON pr.id=NEW.proposal_id WHERE p.id=NEW.project_id;
  INSERT INTO safe_deals(project_id,assignment_id,customer_user_id,freelancer_user_id,gross_amount_kopecks,platform_fee_kopecks,freelancer_amount_kopecks,fee_rule_version,status,proposal_snapshot,project_snapshot)
  VALUES(NEW.project_id,NEW.id,customer,NEW.freelancer_user_id,NEW.agreed_price_kopecks,fee,NEW.agreed_price_kopecks-fee,fee_rule.version,'AWAITING_FUNDING',proposal,project);
  RETURN NEW;
END $$;
CREATE TRIGGER project_assignment_safe_deal AFTER INSERT ON project_assignments FOR EACH ROW WHEN(NEW.proposal_id IS NOT NULL) EXECUTE FUNCTION create_safe_deal_for_assignment();
