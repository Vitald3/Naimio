-- Keep all independently configurable domains visible to staff from the first
-- deployment. These routes remain unusable until their selected provider is
-- both enabled and configured from the API environment.
INSERT INTO payment_provider_routes(domain,provider)
VALUES ('SAFE_DEAL','yookassa'),('PRO_SUBSCRIPTION','yookassa'),('OTHER_PLATFORM_PAYMENT','yookassa')
ON CONFLICT(domain) DO NOTHING;
