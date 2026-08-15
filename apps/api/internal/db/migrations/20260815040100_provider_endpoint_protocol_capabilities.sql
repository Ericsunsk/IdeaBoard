-- migrate:up
ALTER TABLE provider_endpoints
  ADD COLUMN protocol VARCHAR(80) NOT NULL DEFAULT 'model-api',
  ADD CONSTRAINT chk_provider_endpoints_protocol_not_blank CHECK (btrim(protocol) <> '');

CREATE TABLE provider_endpoint_capabilities (
  provider_endpoint_id BIGINT NOT NULL,
  capability VARCHAR(100) NOT NULL,
  created_at TIMESTAMPTZ(3) NOT NULL DEFAULT now(),
  CONSTRAINT pk_provider_endpoint_capabilities PRIMARY KEY (provider_endpoint_id, capability),
  CONSTRAINT fk_provider_endpoint_capabilities_endpoint_id FOREIGN KEY (provider_endpoint_id) REFERENCES provider_endpoints(id) ON DELETE CASCADE,
  CONSTRAINT chk_provider_endpoint_capabilities_name_not_blank CHECK (btrim(capability) <> '')
);

CREATE INDEX idx_provider_endpoint_capabilities_capability
  ON provider_endpoint_capabilities (capability, provider_endpoint_id);

INSERT INTO provider_endpoint_capabilities (provider_endpoint_id, capability)
SELECT id, 'image-generation' FROM provider_endpoints;

-- migrate:down
DROP TABLE provider_endpoint_capabilities;
ALTER TABLE provider_endpoints
  DROP CONSTRAINT chk_provider_endpoints_protocol_not_blank,
  DROP COLUMN protocol;
