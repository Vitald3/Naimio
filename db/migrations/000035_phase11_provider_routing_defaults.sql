-- The provider catalogue is non-secret and lets an administrator enable only
-- adapters whose credentials are present in the running API process.
INSERT INTO payment_provider_settings(provider,enabled,environment)
VALUES ('yookassa',false,'sandbox'),('tbank',false,'sandbox'),('yandex_pay',false,'sandbox'),
       ('cloudpayments',false,'sandbox'),('robokassa',false,'sandbox')
ON CONFLICT(provider) DO NOTHING;
