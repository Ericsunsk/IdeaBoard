-- Branding stores the object key independently of the public delivery URL.
UPDATE system_configs
SET value = substr(value, 2)
WHERE config_key = 'branding_logo_url'
  AND value LIKE '/uploads/site/assets/%';
