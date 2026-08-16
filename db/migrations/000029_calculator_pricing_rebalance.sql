UPDATE calculator_definitions SET pricing_config = jsonb_set(jsonb_set(jsonb_set(jsonb_set(pricing_config,
  '{baseline_min_kopecks}', '3500000'::jsonb), '{baseline_max_kopecks}', '8000000'::jsonb),
  '{option_basis_points}', '{"complexity":{"basic":0,"workflow":2500,"commerce":5000}}'::jsonb),
  '{number_basis_points}', '{"integrations":500}'::jsonb), updated_at=now(), version=version+1
WHERE slug='telegram-bot' AND enabled=true;

UPDATE calculator_definitions SET pricing_config = jsonb_set(jsonb_set(jsonb_set(jsonb_set(pricing_config,
  '{baseline_min_kopecks}', '2500000'::jsonb), '{baseline_max_kopecks}', '6000000'::jsonb),
  '{option_basis_points}', '{"design":{"template":0,"custom":4000}}'::jsonb),
  '{number_basis_points}', '{"sections":200}'::jsonb), updated_at=now(), version=version+1
WHERE slug='landing-page' AND enabled=true;

UPDATE calculator_definitions SET pricing_config = jsonb_set(jsonb_set(jsonb_set(jsonb_set(pricing_config,
  '{baseline_min_kopecks}', '2500000'::jsonb), '{baseline_max_kopecks}', '6000000'::jsonb),
  '{option_basis_points}', '{"site_size":{"small":0,"medium":2500,"large":6000},"competition":{"low":0,"medium":2000,"high":5000}}'::jsonb),
  '{boolean_basis_points}', '{"content":2500}'::jsonb), updated_at=now(), version=version+1
WHERE slug='seo' AND enabled=true;
