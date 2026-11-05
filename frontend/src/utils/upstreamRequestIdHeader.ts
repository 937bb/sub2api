// Match the backend's optional HTTP header-name validation before OAuth starts.
export function isValidUpstreamRequestIdHeader(value: string): boolean {
  const name = value.trim()
  return name === '' || (name.length <= 64 && /^[!#$%&'*+.^_`|~0-9A-Za-z-]+$/.test(name))
}
