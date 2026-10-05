import type { EvaluateRequest } from '../../bindings/poe2filter/internal/trade/models'

type SearchIdentity = { name: string; baseType: string; rarity: string }

// The untouched backend draft has no status. A transferred request does,
// and its empty name/base/rarity fields are deliberate search choices.
export function marketItemFromDraft<T extends SearchIdentity>(item: T, draft?: EvaluateRequest): T {
  if (!draft?.status) return item
  return { ...item, name: draft.name, baseType: draft.baseType, rarity: draft.rarity }
}

// Clearing the item box removes only its identity, preserving stats/category.
export function editMarketIdentity<T extends SearchIdentity>(item: T, query: string): T {
  return query.trim() ? item : { ...item, name: '', baseType: '' }
}
