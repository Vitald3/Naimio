INSERT INTO feature_flags(key, enabled, description, config)
VALUES (
  'site_appearance',
  true,
  'Public project identity, support contacts and visual theme.',
  '{"project_name":"Naimio","project_description":"Маркетплейс профессиональных услуг","support_email":"","support_phone":"","legal_company_name":"","footer_copyright":"© Naimio","primary_button_color":"#15956a","button_hover_color":"#0d7452","green_heading_color":"#0d7452","bright_blue_color":"#2563a7","heading_color":"#0d1f16","body_text_color":"#13261d","page_background_color":"#ffffff"}'::jsonb
)
ON CONFLICT (key) DO NOTHING;
