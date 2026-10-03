import { t } from './i18n.mjs';
import { candidates, chooseWeighted, roll } from './engine.mjs';

// Corruption (Vaal Orb) and Sanctification (Divine Orb with Omen of
// Sanctification). Both lock the item: no currency works on it afterwards.
//
// Vaal Orb on non-unique equipment, as the user confirmed in game (2026-10-03),
// one outcome with equal odds among those the item can take:
//   nothing   - only the Corrupted mark
//   reroll    - 1 to 3 affixes are replaced by new random ones
//   scale     - every affix's values are multiplied, each by its own
//               0.78-1.22 (0.01 steps), rounded to the nearest
//   socket    - one more augment socket, past the normal limit (armour,
//               martial weapons); wands and staves gain quality up to 23%
//               instead; jewellery and quivers have neither
//   enchant   - a corruption enchantment; the game gives them no weights,
//               so each of the class's enchantments is equally likely
// Omen of Corruption no longer exists. A fractured affix keeps its place
// and values, as with a Divine Orb.

export const isLocked = item => Boolean(item.corrupted || item.sanctified);
export const lockReason = item => item.sanctified ? t('err.sanctified') : item.corrupted ? t('err.corrupted') : '';

// A random magnitude multiplier, 0.78 to 1.22 in 0.01 steps (45 values).
export const multiplier = (random = Math.random) => (78 + Math.floor(random() * 45)) / 100;

// scaleValues multiplies a modifier's rolled values; whole-number ranges stay
// whole (Sanctify rounds up, corruption to the nearest), decimal ones keep two
// digits.
export function scaleValues(mod, factor, up = false) {
  const values = (mod.values || []).map((value, i) => {
    const range = mod.ranges?.[i] || {};
    const whole = Number.isInteger(range.min) && Number.isInteger(range.max);
    const scaled = value * factor;
    if (whole) return up ? Math.ceil(scaled - 1e-9) : Math.round(scaled);
    return Number((up ? Math.ceil(scaled * 100 - 1e-9) / 100 : scaled).toFixed(2));
  });
  return { ...mod, values };
}

const scaleAll = (item, random, up) => ({ ...item,
  mods: item.mods.map(mod => mod.fractured || mod.unrevealed ? mod : scaleValues(mod, multiplier(random), up)) });

export const enchantRows = data => data.mods.filter(m => m.pool === 'corrupted' && m.affix === 'Enchant');
export const quality = { wandsStaves: new Set(['wand', 'staff']), max: 23 };

// corruptOutcomes lists what a Vaal Orb can do to this item. classId is the
// item class ('gloves', 'wand' …), sockets/maxSockets its augment sockets.
export function corruptOutcomes(item, data, { classId, sockets = 0, maxSockets = 0, quality: current = 20 } = {}) {
  const out = ['nothing'];
  const loose = item.mods.filter(m => !m.fractured && !m.unrevealed);
  if (loose.length) out.push('reroll', 'scale');
  if (quality.wandsStaves.has(classId)) { if (current < quality.max) out.push('quality'); }
  else if (maxSockets > 0 && sockets < maxSockets) out.push('socket');
  if (!item.enchant && enchantRows(data).length) out.push('enchant');
  return out;
}

export function corruptReason(item) {
  if (item.reveal) return t('err.pendingReveal');
  if (isLocked(item)) return lockReason(item);
  if (!['Normal', 'Magic', 'Rare'].includes(item.rarity)) return t('err.badRarity');
  return '';
}

// corrupt applies one Vaal Orb and returns the new item and the outcome.
export function corrupt(item, data, context = {}, random = Math.random) {
  const reason = corruptReason(item);
  if (reason) throw new Error(reason);
  const outcomes = corruptOutcomes(item, data, context);
  const outcome = outcomes[Math.floor(random() * outcomes.length)];
  let next = { ...item, mods: [...item.mods], corrupted: true }, detail = 0;
  if (outcome === 'reroll') {
    // Each replaced affix is a removal and a fresh roll from the normal pool
    // (with the socketed runes' pools), like a Chaos Orb without its rules
    // on which side; fractured and unrevealed affixes stay.
    const want = 1 + Math.floor(random() * 3);
    for (let i = 0; i < want; i++) {
      const options = next.mods.map((m, index) => ({ m, index })).filter(({ m }) => !m.fractured && !m.unrevealed);
      if (!options.length) break;
      const { index } = options[Math.floor(random() * options.length)];
      const without = { ...next, mods: next.mods.filter((_, j) => j !== index) };
      const rows = candidates(without, data, { rarity: next.rarity });
      if (!rows.length) break;
      next = { ...without, mods: [...without.mods, roll(chooseWeighted(rows, random), random)] };
      detail++;
    }
  } else if (outcome === 'scale') {
    next = scaleAll(next, random, false);
  } else if (outcome === 'socket') {
    next.sockets = (context.sockets || 0) + 1;
    detail = next.sockets;
  } else if (outcome === 'quality') {
    next.quality = Math.min(quality.max, (context.quality ?? 20) + 1 + Math.floor(random() * 3));
    detail = next.quality;
  } else if (outcome === 'enchant') {
    const rows = enchantRows(data);
    next.enchant = roll(rows[Math.floor(random() * rows.length)], random);
  }
  return { item: next, outcome, detail, odds: 1 / outcomes.length };
}

// Sanctify: a Divine Orb with Omen of Sanctification on a Rare. Every affix's
// current values are multiplied, each by its own 0.78-1.22, rounded up, and
// the item can no longer be modified.
export function sanctifyReason(item) {
  if (item.reveal) return t('err.pendingReveal');
  if (isLocked(item)) return lockReason(item);
  if (item.rarity !== 'Rare') return t('err.needRarity', 'Rare');
  if (!item.mods.some(m => !m.fractured && !m.unrevealed)) return item.mods.length ? t('err.onlyFractured') : t('err.noMods');
  return '';
}
export function sanctify(item, random = Math.random) {
  const reason = sanctifyReason(item);
  if (reason) throw new Error(reason);
  return { ...scaleAll(item, random, true), sanctified: true };
}
