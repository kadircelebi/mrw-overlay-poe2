import { t } from './i18n.mjs';
import { maxQuality } from './infuser.mjs';
// Breach catalysts on rings and amulets: each adds 1% quality of one
// modifier type up to the item's maximum (20%, more on Breach Rings and with
// Essence of the Breach: infuser.mjs; checked in game) and replaces any other
// type.
// That quality raises the matching modifiers' values (+100 Life is +120 at
// 20%, rounded down: checked in game) and, with an Omen of Catalysing
// Exaltation, the next Exalted Orb's chance of a matching modifier.
//
// The chance: GGG publishes no formula. Craft of Exile's calculator weights
// matching modifiers by x(1 + 0.2 x quality), +0.12 per point over 20; it is
// used here as an estimate. Assumed, not checked: another catalyst keeps the
// quality amount and changes its type, as in Path of Exile 1; with Greater
// Exaltation both added modifiers are boosted before the quality is used up.

const art = name => `Art/2DItems/Currency/Breach/BreachCatalyst${name}.webp`;
export const catalystTypes = {
  'flesh-catalyst': { name: 'Flesh Catalyst', label: 'Life', tags: ['life'], icon: art('Life') },
  'neural-catalyst': { name: 'Neural Catalyst', label: 'Mana', tags: ['mana'], icon: art('Mana') },
  'carapace-catalyst': { name: 'Carapace Catalyst', label: 'Defence', tags: ['defences'], icon: art('Defences') },
  'uul-netols-catalyst': { name: "Uul-Netol's Catalyst", label: 'Physical', tags: ['physical'], icon: art('Physical') },
  'xophs-catalyst': { name: "Xoph's Catalyst", label: 'Fire', tags: ['fire'], icon: art('Fire') },
  'tuls-catalyst': { name: "Tul's Catalyst", label: 'Cold', tags: ['cold'], icon: art('Cold') },
  'eshs-catalyst': { name: "Esh's Catalyst", label: 'Lightning', tags: ['lightning'], icon: art('Lightning') },
  'chayulas-catalyst': { name: "Chayula's Catalyst", label: 'Chaos', tags: ['chaos'], icon: art('Chaos') },
  'reaver-catalyst': { name: 'Reaver Catalyst', label: 'Attack', tags: ['attack'], icon: art('Attack') },
  'sibilant-catalyst': { name: 'Sibilant Catalyst', label: 'Caster', tags: ['caster'], icon: art('Caster') },
  'skittering-catalyst': { name: 'Skittering Catalyst', label: 'Speed', tags: ['speed'], icon: art('Speed') },
  'adaptive-catalyst': { name: 'Adaptive Catalyst', label: 'Attribute', tags: ['attribute'], icon: art('Attribute') },
  'necrotic-catalyst': { name: 'Necrotic Catalyst', label: 'Minion', tags: ['minion'], icon: art('Necrotic') },
};
export const catalystClasses = ['Ring', 'Amulet'];
// The highest catalyst quality any ring reaches (Refined Breach Ring 45 +
// Essence of the Breach 20 + Infusers 10).
export const catalystMax = 75;
export const isCatalyst = id => Object.hasOwn(catalystTypes, id);

// Catalysts as currency rules, so the board, holding, the ledger and prices
// treat them like any orb.
export const catalystRules = () => Object.fromEntries(Object.entries(catalystTypes).map(([id, c]) => [id, {
  name: c.name, short: c.label, icon: c.icon, tier: 'Catalyst', type: 'Currency',
  beforeRarity: ['Normal', 'Magic', 'Rare'], afterTrigger: 'catalyst', operation: 'catalyst',
}]));

export function catalystReason(item, data, id) {
  if (!catalystClasses.includes(data.options?.ItemClassesCode)) return t('err.catalystClass');
  if (item.catalyst?.id === id && item.catalyst.quality >= maxQuality(item)) return t('err.catalystMax', maxQuality(item));
  return '';
}

export function applyCatalyst(item, id) {
  const quality = Math.min(maxQuality(item), (item.catalyst?.quality || 0) + 1);
  return { ...item, catalyst: { id, quality } };
}

export const catalystMultiplier = q => 1 + 0.2 * Math.min(q, 20) + 0.12 * Math.max(q - 20, 0);
export const catalystMatches = (mod, id) => Boolean(catalystTypes[id]?.tags.some(tag => mod.tags?.includes(tag)));

// The rows' weights as the Omen of Catalysing Exaltation makes them.
export function catalystBoost(item, rows) {
  const c = item.catalyst;
  if (!c?.quality) return rows;
  const m = catalystMultiplier(c.quality);
  return rows.map(row => catalystMatches(row, c.id) ? { ...row, weight: row.weight * m } : row);
}

// A matching modifier's values as the game shows them with the quality, or
// null when the quality does not touch it.
export function augmentedValues(mod, item) {
  const c = item.catalyst;
  if (!c?.quality || !catalystMatches(mod, c.id) || !mod.values) return null;
  const k = 1 + c.quality / 100;
  return mod.values.map(v => Number.isInteger(v) ? Math.floor(v * k) : Math.round(v * k * 100) / 100);
}

// Skill levels are fixed in the text ("+3 ..."), not variable roll ranges.
// Derive their displayed value without changing the saved tier or base text.
export function augmentedMod(mod, item) {
  const values = augmentedValues(mod, item);
  if (!values) return null;
  if (values.length) return { ...mod, values };
  const level = /^\+(\d+)( to Level of all(?: [^\n]+)? Skills)$/.exec(mod.text || '');
  if (!level) return null;
  const raised = Math.floor(Number(level[1]) * (1 + item.catalyst.quality / 100));
  return { ...mod, values, text: `+${raised}${level[2]}` };
}

// "Quality (Life Modifiers): +20%" back to the catalyst, for copied items.
export function catalystFromCopy(raw) {
  const m = /^Quality \(([^)]+) Modifiers\): \+(\d+)%/m.exec(raw || '');
  if (!m) return null;
  const words = m[1].toLowerCase();
  const entry = Object.entries(catalystTypes).find(([, c]) => words.includes(c.label.toLowerCase()) ||
    (c.label === 'Defence' && /armour|evasion|energy/.test(words)));
  return entry ? { id: entry[0], quality: Math.min(catalystMax, Number(m[2])) } : null;
}
