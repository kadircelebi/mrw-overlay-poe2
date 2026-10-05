// Generated locally from the English trade catalogue; no third-party regex data.
export type RegexChoice = { selected: boolean; min?: number; max?: number; mod: { key: string; statId: string; text: string; type: string } }
export type RegexFilter = { group: string; id: string; min?: number; max?: number; option?: string; input?: string; label?: string }
export type RegexGroup = { type: string; choiceKeys: string[] }
export type RegexResult = { text: string; omitted: string[]; ignoredAffixBounds: boolean; error: 'empty' | 'long' | '' }
const escape = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
const patternCache = new WeakMap<string[], { templates: string[]; patterns: Map<string, string> }>()

export function regexTemplate(text: string): string {
  return text.toLowerCase().replace(/\[([^\]|]+)\|([^\]]+)\]/g, '$2')
    .replace(/\[([^\]]+)\]/g, '$1').replace(/\s*\((local|rune|implicit|enchant)\)/g, '')
    .replace(/[+-]?(?:\d+(?:\.\d+)?|#)/g, '#').replace(/\barea\b/g, 'map').replace(/\s+/g, ' ').trim()
}

// Choose a short literal that distinguishes this modifier from other catalogue
// lines. Anchors distinguish a whole-line modifier from a longer hybrid line.
export function modifierPattern(text: string, catalogue: string[]): string {
  const template = regexTemplate(text)
  let cache = patternCache.get(catalogue)
  if (!cache) { cache = { templates: [...new Set(catalogue.map(regexTemplate))], patterns: new Map() }; patternCache.set(catalogue, cache) }
  const cached = cache.patterns.get(template)
  if (cached) return cached
  const others = cache.templates.filter((s) => s !== template)
  const candidates: string[] = []
  for (const match of template.matchAll(/[^#\n]+/g)) {
    const literal = match[0]
    for (let size = 6; size <= Math.min(literal.length, 32); size++) {
      for (let start = 0; start + size <= literal.length; start++) {
        const part = literal.slice(start, start + size)
        if (part !== part.trim() || !/[a-z]/.test(part)) continue
        const atStart = match.index === 0 && start === 0
        const atEnd = match.index! + start + size === template.length
        if (!others.some((s) => s.includes(part))) candidates.push(escape(part))
        if (atStart && !others.some((s) => s.startsWith(part))) candidates.push('^' + escape(part))
        if (atEnd && !others.some((s) => s.endsWith(part))) candidates.push(escape(part) + '$')
      }
      if (candidates.length) break
    }
  }
  candidates.sort((a, b) => a.length - b.length || a.localeCompare(b))
  const pattern = (candidates[0] ?? template.split('#').map(escape).join('[+-]?\\d+(\\.\\d+)?(\\([^)]*\\))?')).replace(/\bmap\b/g, '(map|area)')
  cache.patterns.set(template, pattern)
  return pattern
}

// Inclusive, exact integer ranges, including transitions such as 99 -> 100.
export function integerRange(min = 0, max = 9999): string | null {
  if (!Number.isInteger(min) || !Number.isInteger(max) || min < 0 || max < min || max > 9999) return null
  function digits(lo: string, hi: string): string[] {
    if (lo === hi) return [lo]
    if (/^0+$/.test(lo) && /^9+$/.test(hi)) return [lo.length === 1 ? '\\d' : `\\d{${lo.length}}`]
    if (lo[0] === hi[0]) return digits(lo.slice(1), hi.slice(1)).map((s) => lo[0] + s)
    if (lo.length === 1) return [`[${lo}-${hi}]`]
    const tail = lo.length - 1
    const a = Number(lo[0]), b = Number(hi[0])
    const out: string[] = []
    const lowPartial = !/^0+$/.test(lo.slice(1)), highPartial = !/^9+$/.test(hi.slice(1))
    if (lowPartial) out.push(...digits(lo.slice(1), '9'.repeat(tail)).map((s) => a + s))
    const from = a + Number(lowPartial), to = b - Number(highPartial)
    if (from <= to) out.push((from === to ? String(from) : `[${from}-${to}]`) + (tail === 1 ? '\\d' : `\\d{${tail}}`))
    if (highPartial) out.push(...digits('0'.repeat(tail), hi.slice(1)).map((s) => b + s))
    return out
  }
  const parts: string[] = []
  for (let length = String(min).length; length <= String(max).length; length++) {
    const lo = Math.max(min, length === 1 ? 0 : 10 ** (length - 1))
    const hi = Math.min(max, 10 ** length - 1)
    if (lo <= hi) parts.push(...digits(String(lo), String(hi)))
  }
  return parts.length === 1 ? parts[0] : `(${parts.join('|')})`
}

function filterRange(min?: number, max?: number): string | null {
  if (max !== undefined) return integerRange(min ?? 0, max)
  const lo = min ?? 0
  const length = String(lo).length
  const bounded = integerRange(lo, 10 ** length - 1)
  if (bounded === null) return null
  return `(${bounded}|[1-9]\\d{${length},})`
}

const numericProperties: Record<string, string> = {
  map_iir: 'item rarity', map_packsize: 'pack size', map_rare_monsters: 'monster rarity',
  map_magic_monsters: 'monster effectiveness', map_bonus: 'waystone drop chance',
  map_revives: 'revives available', map_tier: 'waystone.*tier', ilvl: 'item level',
  quality: 'quality', gem_level: 'level', ar: 'armour', ev: 'evasion rating', es: 'energy shield', spirit: 'spirit', ward: 'runic ward',
}
const states: Record<string, string> = { corrupted: '^corrupted$', twice_corrupted: '^twice corrupted$', mirrored: '^mirrored$', sanctified: '^sanctified$', identified: '^unidentified$' }

export function buildMarketRegex(input: {
  choices: RegexChoice[]; groups: RegexGroup[]; filters: RegexFilter[]; catalogue: string[];
  baseType?: string; name?: string; mode: 'all' | 'any';
}): RegexResult {
  const positive: string[] = [], required: string[] = [], omitted: string[] = []
  let ignoredAffixBounds = false
  const quote = (s: string) => `"${s}"`
  if (input.name) required.push(quote(escape(input.name.toLowerCase())))
  else if (input.baseType) required.push(quote(escape(input.baseType.toLowerCase())))
  for (const choice of input.choices.filter((c) => c.selected)) {
    const groups = input.groups.filter((g) => g.choiceKeys.includes(choice.mod.key))
    if (!groups.length) continue
    if (groups.some((g) => !['and', 'not', 'count'].includes(g.type))) { omitted.push(choice.mod.text); continue }
    const uses = /(?:#|\d+(?:\([^)]*\))?)\s+uses remaining\b/i.test(choice.mod.text)
    const min = typeof choice.min === 'number' && Number.isFinite(choice.min) ? choice.min : undefined
    const max = typeof choice.max === 'number' && Number.isFinite(choice.max) ? choice.max : undefined
    if (choice.mod.type === 'pseudo' && !uses) { omitted.push(choice.mod.text); continue }
    let pattern = uses ? 'uses remaining' : modifierPattern(choice.mod.text, input.catalogue)
    if (uses && (min !== undefined || max !== undefined)) {
      const range = filterRange(min, max)
      if (range === null) { omitted.push(choice.mod.text); continue }
      pattern = `^${range}(\\([^)]*\\))? uses remaining`
    } else if (min !== undefined || max !== undefined) {
      // Affix numbers were deliberately excluded from this feature's scope.
      ignoredAffixBounds = true
    }
    // Remaining uses is an item condition, independent of affix all/any.
    if (uses) required.push(quote((groups.every((g) => g.type === 'not') ? '!' : '') + pattern))
    else if (groups.every((g) => g.type === 'not')) required.push(quote('!' + pattern))
    else positive.push(pattern)
  }
  for (const filter of input.filters) {
    if (filter.option === 'any') continue
    if (filter.id === 'rarity') {
      if (['normal', 'magic', 'rare', 'unique'].includes(filter.option ?? '')) required.push(quote('rarity: ' + filter.option))
      else omitted.push(filter.label ?? filter.id)
      continue
    }
    if (filter.id === 'category') { if (!input.baseType && !input.name) omitted.push(filter.label ?? filter.id); continue }
    const state = filter.group === 'misc_filters' ? states[filter.id] : undefined
    if (state && ['true', 'false'].includes(filter.option ?? '')) {
      const negative = filter.id === 'identified' ? filter.option === 'true' : filter.option === 'false'
      required.push(quote((negative ? '!' : '') + state))
      continue
    }
    const property = numericProperties[filter.id]
    if (property && (filter.min !== undefined || filter.max !== undefined) && !filter.option && !filter.input) {
      const range = filterRange(filter.min, filter.max)
      if (range) required.push(quote(filter.id === 'map_tier' ? `tier ${range}\\)` : `${property}: \\+?${range}(\\([^)]*\\))?(%|$)`))
      else omitted.push(filter.label ?? filter.id)
    } else omitted.push(filter.label ?? filter.id)
  }
  const unique = [...new Set(positive)]
  const clauses = [...new Set(required)]
  if (unique.length) clauses.push(...(input.mode === 'all' ? unique.map(quote) : [quote(unique.join('|'))]))
  const text = clauses.join(' ')
  return { text, omitted: [...new Set(omitted)], ignoredAffixBounds, error: !text ? 'empty' : text.length > 250 ? 'long' : '' }
}
