import type { EvaluatedListing } from '../../bindings/poe2filter/internal/trade/models'

function canonical(value: unknown): string {
  if (Array.isArray(value)) return '[' + value.map(canonical).join(',') + ']'
  if (value && typeof value === 'object') {
    return '{' + Object.entries(value).filter(([, v]) => v !== undefined).sort(([a], [b]) => a.localeCompare(b))
      .map(([k, v]) => JSON.stringify(k) + ':' + canonical(v)).join(',') + '}'
  }
  return JSON.stringify(value) ?? 'null'
}

// Listing ids, indexed dates, travel tokens and image URLs do not distinguish
// equivalent items. Keep every actual item attribute, including rolls and ilvl.
export function uniqueListings(listings: EvaluatedListing[]): EvaluatedListing[] {
  const seen = new Set<string>()
  return listings.filter((row) => {
    if (!row.account?.trim() || !row.currency || !Number.isFinite(row.amount) || !row.item?.baseType) return true
    const { icon: _icon, properties, mods, ...attributes } = row.item
    const item = {
      ...attributes,
      properties: (properties ?? []).map(canonical).sort(),
      mods: (mods ?? []).map((mod) => ({ ...mod, parts: (mod.parts ?? []).map(canonical).sort() })).map(canonical).sort(),
    }
    const key = canonical([row.account.trim().toLowerCase(), row.currency, row.amount, item])
    if (seen.has(key)) return false
    seen.add(key)
    return true
  })
}
