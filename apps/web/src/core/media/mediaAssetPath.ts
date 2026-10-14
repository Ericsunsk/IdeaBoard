/** Matches byte-delivery routes by URL.pathname; metadata and unknown tails are excluded. */
export function isMediaAssetDeliveryPath(pathname: string): boolean {
  return /^\/api\/v1\/workspaces\/[^/\s?#]+\/media-assets\/[^/\s?#]+\/(?:content|derivatives\/[^/\s?#]+)$/.test(
    pathname,
  )
}
