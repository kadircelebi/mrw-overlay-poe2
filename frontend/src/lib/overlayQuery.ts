import type { Item, ItemMod } from '../../bindings/poe2filter/internal/overlay/models'
import type { EvaluateRequest, SelectedFilter, SelectedStat, SelectedStatGroup } from '../../bindings/poe2filter/internal/trade/models'
import { t } from './i18n.svelte'

export type ModChoice = {
  mod: ItemMod
  selected: boolean
  min?: number
  max?: number
}

// ItemToggles are the parts of the item title the user can click out of a
// search. When the base type is off, the item class narrows it instead.
// rarity overrides the item's own rarity; '' searches any rarity.
export type ItemToggles = { name: boolean; base: boolean; rarity?: string }

export const allOn: ItemToggles = { name: true, base: true }

export const rarityOptions = [
  { id: '', text: 'Any' }, { id: 'normal', text: 'Normal' }, { id: 'magic', text: 'Magic' }, { id: 'rare', text: 'Rare' },
  { id: 'unique', text: 'Unique' }, { id: 'uniquefoil', text: 'Unique (Foil)' }, { id: 'nonunique', text: 'Any Non-Unique' },
]

// PropertyFilter is a clickable item property (defences, spirit, sockets).
// base is the item's own value; Broad mode lowers min from it like affixes.
export type PropertyFilter = { group: string; id: string; name: string; value: string; enabled: boolean; base?: number; min?: number; max?: number }

// Trade filters for the properties the game prints above the modifiers.
const propertyFilterIDs: Record<string, { group: string; id: string }> = {
  Armour: { group: 'equipment_filters', id: 'ar' }, 'Evasion Rating': { group: 'equipment_filters', id: 'ev' },
  'Energy Shield': { group: 'equipment_filters', id: 'es' }, Spirit: { group: 'equipment_filters', id: 'spirit' },
  'Runic Ward': { group: 'equipment_filters', id: 'ward' }, DPS: { group: 'equipment_filters', id: 'dps' },
  'Physical DPS': { group: 'equipment_filters', id: 'pdps' }, 'Elemental DPS': { group: 'equipment_filters', id: 'edps' },
  'Critical Hit Chance': { group: 'equipment_filters', id: 'crit' }, 'Attacks per Second': { group: 'equipment_filters', id: 'aps' },
  // Waystone totals ("Endgame Filters" on the trade site).
  'Item Rarity': { group: 'map_filters', id: 'map_iir' }, 'Pack Size': { group: 'map_filters', id: 'map_packsize' },
  'Monster Rarity': { group: 'map_filters', id: 'map_rare_monsters' }, 'Monster Effectiveness': { group: 'map_filters', id: 'map_magic_monsters' },
  'Waystone Drop Chance': { group: 'map_filters', id: 'map_bonus' }, 'Revives Available': { group: 'map_filters', id: 'map_revives' },
}

// Damage is what a weapon is priced by, and a waystone by its totals, so
// those filters start switched on; its modifiers start off (see the parser).
const enabledByDefault = new Set(['pdps', 'edps', 'map_iir', 'map_packsize', 'map_rare_monsters', 'map_magic_monsters', 'map_bonus'])

export function propertyFiltersFor(item: Item): PropertyFilter[] {
  const out: PropertyFilter[] = []
  for (const prop of item.properties ?? []) {
    const filter = propertyFilterIDs[prop.name]
    if (!filter) continue
    const value = parseFloat(prop.value)
    const base = Number.isFinite(value) ? value : undefined
    out.push({ ...filter, name: prop.name, value: prop.value, enabled: enabledByDefault.has(filter.id), base, min: base })
  }
  if (item.runeSockets > 0) {
    // An exceptional item is priced by its extra sockets, so they are searched.
    out.push({ group: 'equipment_filters', id: 'rune_sockets', name: 'Rune Sockets', value: String(item.runeSockets), enabled: item.exceptional, min: item.runeSockets })
  }
  // DPS first: it is what the eye looks for on a weapon.
  const order = ['dps', 'pdps', 'edps']
  return out.sort((a, b) => (order.indexOf(a.id) + 1 || 99) - (order.indexOf(b.id) + 1 || 99))
}

// Trade categories by the item class the game prints in "Item Class:".
const categories: Record<string, string> = {
  Claws: 'weapon.claw', Daggers: 'weapon.dagger', 'One Hand Swords': 'weapon.onesword', 'One Hand Axes': 'weapon.oneaxe',
  'One Hand Maces': 'weapon.onemace', Spears: 'weapon.spear', Flails: 'weapon.flail', 'Two Hand Swords': 'weapon.twosword',
  'Two Hand Axes': 'weapon.twoaxe', 'Two Hand Maces': 'weapon.twomace', Quarterstaves: 'weapon.warstaff', Talismans: 'weapon.talisman',
  Bows: 'weapon.bow', Crossbows: 'weapon.crossbow', Wands: 'weapon.wand', Sceptres: 'weapon.sceptre', Staves: 'weapon.staff',
  Helmets: 'armour.helmet', 'Body Armours': 'armour.chest', Gloves: 'armour.gloves', Boots: 'armour.boots', Quivers: 'armour.quiver',
  Shields: 'armour.shield', Foci: 'armour.focus', Bucklers: 'armour.buckler', Amulets: 'accessory.amulet', Belts: 'accessory.belt',
  Rings: 'accessory.ring', Jewels: 'jewel', Charms: 'flask.charm', 'Life Flasks': 'flask.life', 'Mana Flasks': 'flask.mana',
  Relics: 'sanctum.relic', Waystones: 'map.waystone', Tablets: 'map.tablet',
}

// searchLabel is what identifies an item on the trade site: a rare's name is a
// random affix pair the game made up, so only a unique's name is meaningful.
export function searchLabel(item: { rarity: string; name: string; baseType: string }): string {
  return item.rarity === 'unique' && item.name ? item.name : item.baseType
}

export function categoryFor(itemClass: string): string {
  return categories[itemClass] ?? ''
}

// modValue is the number the trade site compares for a modifier. "Adds X to Y
// Damage" is filtered by the average of X and Y, not by X.
export function modValue(mod: ItemMod): number | undefined {
  const values = mod.values ?? []
  if (values.length === 2 && /^Adds .+ to .+ Damage/i.test(mod.text)) return round((values[0] + values[1]) / 2)
  return values[0]
}

export function choicesFor(item: Item, broad = true): ModChoice[] {
  return (item.mods ?? []).map((mod) => {
    const current = modValue(mod)
    let min = current
    // An empty slot count is a count, not a roll: Broad does not lower it.
    if (broad && current !== undefined && mod.type !== 'pseudo') min = round(current >= 0 ? current * 0.9 : current * 1.1)
    return { mod, selected: mod.selected && !!mod.statId, min }
  })
}

export function buildRequest(
  item: Item,
  choices: ModChoice[],
  status = 'securable',
  filters: SelectedFilter[] = [],
  groups: SelectedStatGroup[] = [],
  toggles: ItemToggles = allOn,
): EvaluateRequest {
  const stats: SelectedStat[] = choices
    .filter((choice) => choice.selected && choice.mod.statId)
    .map((choice) => ({ id: choice.mod.statId, min: finite(choice.min), max: finite(choice.max) }))
  const category = toggles.base ? '' : categoryFor(item.class)
  if (category && !filters.some((filter) => filter.group === 'type_filters' && filter.id === 'category')) {
    filters = [...filters, { group: 'type_filters', id: 'category', option: category }]
  }
  if (!groups.length) groups = twinGroups(choices)
  return {
    league: '',
    name: item.rarity === 'unique' && toggles.name ? item.name : '',
    baseType: toggles.base ? item.baseType : '',
    rarity: toggles.rarity ?? item.rarity,
    status,
    stats,
    groups,
    filters,
  }
}

// twinGroups puts a line whose wording matches several trade stats (a local
// and a global "increased Armour", two "# to maximum Runic Ward") in a count
// group of all of them with at least one required, as POE2 Overlay does, so
// the search holds whichever the item really has. Without such lines it
// returns no groups and the plain stat list is searched.
export function twinGroups(choices: ModChoice[]): SelectedStatGroup[] {
  const selected = choices.filter((choice) => choice.selected && choice.mod.statId)
  if (!selected.some((choice) => choice.mod.altStatIds?.length)) return []
  const plain: SelectedStat[] = []
  const counts: SelectedStatGroup[] = []
  for (const choice of selected) {
    const min = finite(choice.min), max = finite(choice.max)
    if (!choice.mod.altStatIds?.length) {
      plain.push({ id: choice.mod.statId, min, max })
      continue
    }
    const ids = [choice.mod.statId, ...choice.mod.altStatIds]
    counts.push({ type: 'count', min: 1, stats: ids.map((id) => ({ id, min, max })) })
  }
  return [{ type: 'and', stats: plain }, ...counts].filter((group) => (group.stats?.length ?? 0) > 0)
}

export function round(value: number): number {
  return Math.round(value * 100) / 100
}

function finite(value: number | undefined): number | undefined {
  return typeof value === 'number' && Number.isFinite(value) ? value : undefined
}

export function resetPropertyRanges(filters: PropertyFilter[], broad: boolean) {
  for (const filter of filters) {
    // Sockets are a count, not a roll: Broad does not loosen them.
    if (filter.base === undefined) continue
    filter.min = round(broad ? filter.base * 0.9 : filter.base)
    filter.max = undefined
  }
}

export function resetChoiceRanges(choices: ModChoice[], broad: boolean) {
  for (const choice of choices) {
    const current = modValue(choice.mod)
    choice.min = current === undefined ? undefined : round(broad ? (current >= 0 ? current * 0.9 : current * 1.1) : current)
    choice.max = undefined
  }
}

export function currencyLabel(id: string): string {
  const labels: Record<string, string> = {
    exalted: 'Exalted Orb', divine: 'Divine Orb', chaos: 'Chaos Orb', annulment: 'Orb of Annulment',
    regal: 'Regal Orb', alchemy: 'Orb of Alchemy', vaal: 'Vaal Orb', mirror: 'Mirror of Kalandra',
  }
  return labels[id] ?? id.replaceAll('-', ' ')
}

export function listedAgo(iso: string): string {
  const when = Date.parse(iso)
  if (!Number.isFinite(when)) return iso
  const mins = Math.max(0, Math.floor((Date.now() - when) / 60000))
  if (mins < 60) return t('ov.ago.m', mins)
  const hours = Math.floor(mins / 60)
  if (hours < 24) return t('ov.ago.h', hours)
  return t('ov.ago.d', Math.floor(hours / 24))
}

// The trade fetch call returns listings ten at a time.
export const PAGE_SIZE = 10

export type SortState = { key: string; dir: 'asc' | 'desc' }
export type SortOption = { key: string; label: string; title?: string }

// Price reads best cheapest first; every other value best highest first.
export function nextSort(current: SortState | null, key: string): SortState {
  if (current?.key === key) return { key, dir: current.dir === 'asc' ? 'desc' : 'asc' }
  return { key, dir: key === 'price' ? 'asc' : 'desc' }
}

// Trade sort keys for the listing properties the overlay shows. The keys were
// checked against the PoE2 trade API; an unknown key fails the search.
const propertySortKeys: Record<string, string> = {
  'DPS': 'dps',
  'Physical DPS': 'pdps',
  'Elemental DPS': 'edps',
  'Attacks per Second': 'aps',
  'Critical Hit Chance': 'crit',
  'Armour': 'ar',
  'Evasion Rating': 'ev',
  'Energy Shield': 'es',
  'Spirit': 'spirit',
  'Quality': 'quality',
  'Block chance': 'block',
  'Runic Ward': 'ward',
  'Item Rarity': 'map_iir',
  'Pack Size': 'map_packsize',
  'Monster Rarity': 'map_rare_monsters',
  'Monster Effectiveness': 'map_magic_monsters',
  'Waystone Drop Chance': 'map_bonus',
  'Revives Available': 'map_revives',
}

export function propertySortKey(name: string) {
  return propertySortKeys[name] ?? ''
}

export function statSortKey(statId: string) {
  return statId ? `stat.${statId}` : ''
}

// The stat ids a request filters on, for tinting those lines on listings.
export function searchedStats(request: EvaluateRequest): string[] {
  const stats = request.groups?.length ? request.groups.flatMap((group) => group.stats ?? []) : request.stats ?? []
  return stats.filter((stat) => stat.id && !stat.disabled).map((stat) => stat.id)
}
