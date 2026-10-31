// Models vertical public seam: sibling verticals consume the gateway
// resource through here, never `models/resource` directly.
export { createAdminGatewayResource } from './resource'
