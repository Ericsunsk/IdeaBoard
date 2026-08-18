UPDATE provider_endpoints
SET api_type = 'self-hosted', updated_at = now()
WHERE protocol IN ('image-url-json-to-image', 'flux-fill-json-image')
  AND api_type IN ('google-ai', 'google-cloud');
