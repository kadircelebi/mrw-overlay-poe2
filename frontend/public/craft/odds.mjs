import { candidates, rollPools, removable, removeMod, isFull, overlaps } from './engine.mjs';
import { filterOmenRows } from './omens.mjs';
import { catalystBoost } from './catalyst.mjs';
import { boneRows } from './special.mjs';

// The chance that one use of the selected currency puts each modifier on the
// item, by source_id, worked out with the same rules the engine rolls with:
//   one add      P(row) = w(row) / Σw over every row the orb can add (both sides)
//   two adds     P(row) = P1(row) + Σ_f≠row P1(f) · P2(row | item + f)
//                (Omen of Greater Exaltation; the second roll skips f's family)
//   remove + add the mean of the add odds over every modifier the removal may
//                hit, each equally likely (Chaos Orb)
//   desecrate    the chance the row is among the up to three options offered
//                at the Well of Souls (see offerOdds)
// The probabilities of one family's tiers add up to the family's chance, as
// a family appears at most once.

const neutral = { side: null, tags: [] };
const add = (map, id, p) => map.set(id, (map.get(id) || 0) + p);
// What a roll leaves behind depends only on the family it hit, so the walks
// below go family by family: P(row) = P(family) · w(row) / w(family).
function families(rows) {
  const groups = new Map();
  for (const row of rows) {
    const key = row.affix + ':' + row.families[0];
    if (!groups.has(key)) groups.set(key, { rows: [], weight: 0, sample: row });
    const group = groups.get(key); group.rows.push(row); group.weight += row.weight;
  }
  return [...groups.values()];
}
const spread = (out, group, p) => { for (const row of group.rows) add(out, row.source_id, p * row.weight / group.weight); };

function weighted(item, data, o) {
  let rows = filterOmenRows(candidates(item, data, { pool: o.pools, minimum: o.minimum, rarity: o.rarity }), o.effects);
  if (o.catalyse) rows = catalystBoost(item, rows);
  return rows;
}

export function addOdds(item, data, o, quantity = 1) {
  const out = new Map(), rows = weighted(item, data, o);
  const total = rows.reduce((sum, row) => sum + row.weight, 0);
  if (!total) return out;
  for (const group of families(rows)) {
    const p = group.weight / total;
    spread(out, group, p);
    if (quantity > 1) {
      for (const [id, q] of addOdds({ ...item, mods: [...item.mods, group.sample] }, data, o, quantity - 1)) add(out, id, p * q);
    }
  }
  return out;
}

// Each removal option is equally likely (engine.removeRandom).
function meanOver(items, odds) {
  const out = new Map();
  for (const next of items) for (const [id, p] of odds(next)) add(out, id, p / items.length);
  return out;
}

// The settings of one orb use: the rows it can add, the lowest modifier level
// (Greater/Perfect), the rarity the limits are counted at and the omens that
// shape the add. Side and tag omens only steer Exalted Orbs (applyOrbOmens).
export function orbOptions(item, id, rule, effects = neutral) {
  const exalted = id.includes('exalted');
  return {
    pools: rollPools(item),
    minimum: rule.beforeMin_mod_lv || 1,
    rarity: rule.afterRarity || item.rarity,
    effects: exalted ? { side: effects.side, tags: effects.tags || [] } : neutral,
    catalyse: exalted && Boolean(effects.catalyse),
    quantity: exalted ? effects.quantity || 1 : 1,
  };
}

export function orbOdds(item, data, id, rule, effects = neutral) {
  const o = orbOptions(item, id, rule, effects);
  if (rule.afterTrigger === 'add') return addOdds(item, data, o, o.quantity);
  if (rule.afterTrigger === 'del_add') {
    const options = removable(item, effects.removal || {}).map(index => removeMod(item, index));
    return meanOver(options, next => addOdds(next, data, o));
  }
  return null;
}

// Up to three options from distinct families, each drawn by weight from what
// the earlier draws left (special.revealOptions). The chance a row is offered
// is the sum over every draw order that reaches it.
export function offerOdds(rows, picks = 3) {
  const out = new Map();
  const walk = (list, left, reach) => {
    const total = list.reduce((sum, group) => sum + group.weight, 0);
    if (!total) return;
    for (const group of list) {
      const p = reach * group.weight / total;
      spread(out, group, p);
      if (left > 1) walk(list.filter(g => !overlaps(g.sample, group.sample)), left - 1, p);
    }
  };
  walk(families(rows), picks, 1);
  return out;
}

// A bone: on a full item one modifier (on the omen's side) goes first; the
// bone then picks the side by one weighted roll, and the options come from
// that side's rows.
export function desecrateOdds(item, data, rule, effects) {
  const starts = isFull(item) ? removable(item, { side: effects.side }).map(index => removeMod(item, index)) : [item];
  return meanOver(starts, start => {
    const rows = boneRows(start, data, rule, effects), out = new Map();
    const total = rows.reduce((sum, row) => sum + row.weight, 0);
    for (const side of ['Prefix', 'Suffix']) {
      const list = rows.filter(m => m.affix === side);
      const share = list.reduce((sum, row) => sum + row.weight, 0) / (total || 1);
      if (share) for (const [id, p] of offerOdds(list)) add(out, id, share * p);
    }
    return out;
  });
}

// The options an unrevealed Desecrated modifier will offer.
export function revealOdds(item, data) {
  const index = item.mods.findIndex(m => m.unrevealed);
  if (index < 0) return null;
  const mod = item.mods[index], ctx = mod.revealWith || { effects: { side: null, tags: [], quantity: 1 }, minimum: 1 };
  return offerOdds(boneRows(removeMod(item, index), data, { minimum: ctx.minimum }, ctx.effects).filter(m => m.affix === mod.affix));
}
