import type { Item, ItemMod } from '../../bindings/poe2filter/internal/overlay/models'
import type { EvaluateRequest, SelectedFilter, SelectedStat, SelectedStatGroup } from '../../bindings/poe2filter/internal/trade/models'

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
export type PropertyFilter = { id: string; name: string; value: string; enabled: boolean; base?: number; min?: number; max?: number }

const propertyFilterIDs: Record<string, string> = {
  Armour: 'ar', 'Evasion Rating': 'ev', 'Energy Shield': 'es', Spirit: 'spirit', 'Runic Ward': 'ward',
  DPS: 'dps', 'Physical DPS': 'pdps', 'Elemental DPS': 'edps', 'Critical Hit Chance': 'crit', 'Attacks per Second': 'aps',
}

// Damage is what a weapon is priced by, so its DPS filters start switched on.
const enabledByDefault = new Set(['pdps', 'edps'])

export function propertyFiltersFor(item: Item): PropertyFilter[] {
  const out: PropertyFilter[] = []
  for (const prop of item.properties ?? []) {
    const id = propertyFilterIDs[prop.name]
    if (!id) continue
    const value = parseFloat(prop.value)
    const base = Number.isFinite(value) ? value : undefined
    out.push({ id, name: prop.name, value: prop.value, enabled: enabledByDefault.has(id), base, min: base })
  }
  if (item.runeSockets > 0) {
    out.push({ id: 'rune_sockets', name: 'Rune Sockets', value: String(item.runeSockets), enabled: false, min: item.runeSockets })
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
    if (broad && current !== undefined) min = round(current >= 0 ? current * 0.9 : current * 1.1)
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
  if (mins < 60) return `${mins}m`
  const hours = Math.floor(mins / 60)
  if (hours < 24) return `${hours}h`
  return `${Math.floor(hours / 24)}d`
}
