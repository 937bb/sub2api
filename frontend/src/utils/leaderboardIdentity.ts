export function maskLeaderboardIdentity(value: string, rank: number): string {
  const trimmed = String(value || '').trim()
  if (/^Anonymous #\d+$/.test(trimmed)) return `Anonymous #${rank}`
  if (/^User #\d+$/.test(trimmed)) return `User #${rank}`

  const at = trimmed.indexOf('@')
  if (at <= 0 || at === trimmed.length - 1) return `User #${rank}`

  const local = trimmed.slice(0, at)
  const domain = trimmed.slice(at + 1)
  if (local.includes('*')) return `${local}@${domain}`

  const characters = Array.from(local)
  let maskedLocal: string
  if (characters.length === 1) maskedLocal = `${characters[0]}***`
  else if (characters.length === 2) maskedLocal = `${characters[0]}***${characters[1]}`
  else if (characters.length <= 4) maskedLocal = `${characters[0]}***${characters.at(-1)}`
  else maskedLocal = `${characters.slice(0, 2).join('')}***${characters.slice(-2).join('')}`

  return `${maskedLocal}@${domain}`
}
