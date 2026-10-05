import type { ExoticEntry } from '../../bindings/poe2filter/internal/filter/models'
import type { ExoticTier } from '../../bindings/poe2filter/models'

// Shared by the Exotic group and Alt+E's "add to Exotic": a modifier entry
// keeps a tier and every better one.

export function range(x: { min: number; max: number }): string {
  return x.min === x.max ? `${x.min}` : `${x.min}–${x.max}`
}

// tiersUpTo are the tiers kept when minTier is the lowest one (T1 is best).
export function tiersUpTo(tiers: ExoticTier[] | null | undefined, minTier: number): ExoticTier[] {
  return (tiers ?? []).filter((x) => x.tier <= minTier)
}

// modLabel is the modifier as the game prints it, with the lowest value
// kept: "19+% increased Cast Speed" (same as exoticModLabel in Go).
export function modLabel(text: string, kept: ExoticTier[]): string {
  const low = kept[kept.length - 1]
  return low && text.includes('#') ? text.replace('#', `${low.min}+`) : text
}

// userKey mirrors filter.UserExoticKey, so an edited entry keeps a unique
// key before the saved config comes back.
export function userKey(e: ExoticEntry): string {
  if (e.kind === 'base') return `user|base|${(e.base ?? '').toLowerCase()}`
  const cls = [...(e.classes ?? [])].sort()
  return `user|mod|${cls.join(',').toLowerCase()}|${`${e.stat ?? ''}|${(e.names ?? []).join(',')}`.toLowerCase()}`
}

// tierOption is a tier's line in a tier picker.
export function tierOption(x: ExoticTier): string {
  return `T${x.tier} · ${range(x)} · ilvl ${x.level} · "${x.name}"`
}
