import { t } from './i18n.mjs';
import { candidates, rollPools, sideLimit } from './engine.mjs';
import { orbOmenReason, applyOrbOmens } from './omens.mjs';

// Chaos simulation: press the chosen Chaos Orb (with its omens) on the item
// again and again, exactly as the craft screen would, until a wanted modifier
// line reaches a wanted tier. Many such runs give how many orbs it takes.

// A family can hold several lines ("all Spell", "Fire Spell" … share one) with
// interleaved tier numbers, so a target is a line: the text with its numbers
// replaced, on one side. Within a line a lower tier number is stronger.
export const lineKey = mod => mod.affix + ':' + mod.text.replace(/\((-?\d+(?:\.\d+)?)[—–](-?\d+(?:\.\d+)?)\)|-?\d+(?:\.\d+)?/g, '#');
export const targetOf = row => ({ key: lineKey(row), affix: row.affix, family: row.families[0], tier: row.tier, text: row.text });
export const isHit = (mod, target) => !mod.unrevealed && lineKey(mod) === target.key && mod.tier <= target.tier;
export const hasTarget = (item, target) => item.mods.some(m => isHit(m, target));

// The rows a run can roll never change, so the large class table is cut down
// once to the pools the item rolls from (the engine filters the rest anyway).
export const simData = (item, data) => ({ ...data,
  mods: data.mods.filter(m => rollPools(item).includes(m.pool) && ['Prefix', 'Suffix'].includes(m.affix) && m.weight > 0) });

// The tiers of the target's line that can ever roll on this item with this orb.
export function reachable(item, data, rule, target) {
  return candidates({ ...item, mods: [] }, data, { minimum: rule.beforeMin_mod_lv || 1, rarity: 'Rare' })
    .filter(row => lineKey(row) === target.key && row.tier <= target.tier);
}

// targetReason says why the simulation cannot start ('' when it can).
export function targetReason(item, data, id, rule, effects, target) {
  if (!target) return t('sim.noTarget');
  if (rule.afterTrigger !== 'del_add') return t('sim.notChaos');
  const reason = orbOmenReason(item, data, id, rule, effects);
  if (reason) return reason;
  if (hasTarget(item, target)) return t('sim.already');
  // A fractured modifier is never removed: one of the same family blocks the
  // line for good, and a side full of them leaves no room.
  if (item.mods.some(m => m.fractured && m.affix === target.affix && m.families.includes(target.family))) return t('sim.blockedFractured');
  if (item.mods.filter(m => m.fractured && m.affix === target.affix).length >= sideLimit(item, target.affix, 'Rare')) return t('sim.sideLocked');
  if (!reachable(item, data, rule, target).length) return t('sim.unreachable');
  return '';
}

// runOnce presses orbs until the target is on the item, or cap orbs, or the
// orb can no longer be used (an omen with nothing left to remove).
export function runOnce(item, data, id, rule, effects, target, { random = Math.random, cap = 20000, onStep } = {}) {
  let state = item, orbs = 0;
  while (!hasTarget(state, target)) {
    if (orbs >= cap) return { orbs, item: state, capped: true };
    try { state = applyOrbOmens(state, data, id, rule, effects, random); }
    catch { return { orbs, item: state, stuck: true }; }
    orbs++;
    onStep?.(state, orbs);
  }
  return { orbs, item: state };
}

// fastRunner is runOnce for statistics: the same rules (engine removable +
// candidates + chooseWeighted) without building item objects, so tens of
// thousands of runs fit in a few seconds. The pool is prepared once; each
// press only marks the rows the current modifiers block. Values are not
// rolled. It supports the removal omens a Chaos Orb takes (side, lowest).
export function fastRunner(item, data, rule, effects, target) {
  const removal = effects.removal || {};
  const minimum = rule.beforeMin_mod_lv || 1;
  const pool = simData(item, data).mods;
  const highest = new Map();
  for (const row of pool) {
    const key = row.affix + ':' + row.families[0];
    highest.set(key, Math.max(highest.get(key) || 0, row.required_ilvl));
  }
  const rows = pool.filter(m => m.required_ilvl <= item.ilvl &&
    (m.required_ilvl >= minimum || m.required_ilvl === highest.get(m.affix + ':' + m.families[0])));
  const sides = ['Prefix', 'Suffix'].map(side => {
    const list = rows.filter(r => r.affix === side), cum = [];
    let sum = 0;
    for (const r of list) { sum += r.weight; cum.push(sum); }
    return { side, list, cum, total: sum, limit: sideLimit(item, side, 'Rare') };
  });
  // blocked: the rows of one side that share a family with a modifier.
  const blockCache = new Map();
  const blocks = mod => {
    const key = mod.affix + ':' + mod.families.join('|');
    if (!blockCache.has(key)) {
      const s = sides[mod.affix === 'Prefix' ? 0 : 1];
      blockCache.set(key, s.list.map((r, i) => r.families.some(f => mod.families.includes(f)) ? i : -1).filter(i => i >= 0));
    }
    return blockCache.get(key);
  };
  const slot = mod => ({ affix: mod.affix, families: mod.families, level: mod.required_ilvl,
    fractured: Boolean(mod.fractured), unrevealed: Boolean(mod.unrevealed),
    hit: isHit(mod, target), blocked: blocks(mod) });
  const start = item.mods.map(slot);
  const rowSlot = sides.map(s => s.list.map(r => slot(r)));
  const marks = sides.map(s => new Uint32Array(s.list.length));
  let stamp = 0;

  return function run(random = Math.random, cap = 20000) {
    const mods = start.slice();
    for (let orbs = 0; orbs < cap; orbs++) {
      // Removal (engine removable): never fractured, the omen's side, and with
      // Whittling only the revealed modifiers of the lowest level.
      let options = [];
      for (let i = 0; i < mods.length; i++) {
        const m = mods[i];
        if (!m.fractured && (!removal.side || m.affix === removal.side)) options.push(i);
      }
      if (removal.lowest) {
        options = options.filter(i => !mods[i].unrevealed);
        let low = Infinity;
        for (const i of options) low = Math.min(low, mods[i].level);
        options = options.filter(i => mods[i].level === low);
      }
      if (!options.length) return { orbs, stuck: true };
      mods.splice(options[Math.floor(random() * options.length)], 1);
      // Addition (engine candidates + chooseWeighted): weight over the rows of
      // the sides with room, minus the families already on the item.
      stamp++;
      const free = [0, 0], open = [0, 0];
      for (const m of mods) free[m.affix === 'Prefix' ? 0 : 1]++;
      for (let s = 0; s < 2; s++) {
        if (free[s] >= sides[s].limit) continue;
        let blockedWeight = 0;
        for (const m of mods) {
          if ((m.affix === 'Prefix' ? 0 : 1) !== s) continue;
          for (const i of m.blocked) if (marks[s][i] !== stamp) { marks[s][i] = stamp; blockedWeight += sides[s].list[i].weight; }
        }
        open[s] = sides[s].total - blockedWeight;
      }
      const total = open[0] + open[1];
      if (total <= 0) return { orbs, stuck: true };
      // A full side has no open weight, so it is never drawn.
      let s = random() * total < open[0] ? 0 : 1, pick;
      // Rejection sampling over the side's whole pool gives the same odds as
      // drawing from the unblocked rows only.
      do {
        const needle = random() * sides[s].total, cum = sides[s].cum;
        let lo = 0, hi = cum.length - 1;
        while (lo < hi) { const mid = (lo + hi) >> 1; if (cum[mid] > needle) hi = mid; else lo = mid + 1; }
        pick = lo;
      } while (marks[s][pick] === stamp);
      const added = rowSlot[s][pick];
      mods.push(added);
      if (added.hit) return { orbs: orbs + 1 };
    }
    return { orbs: cap, capped: true };
  };
}

// stats summarises the orb counts of the runs that reached the target.
export function stats(counts) {
  const sorted = [...counts].sort((a, b) => a - b), n = sorted.length;
  if (!n) return null;
  const at = q => sorted[Math.min(n - 1, Math.ceil(q * n) - 1)];
  return { runs: n, mean: sorted.reduce((s, v) => s + v, 0) / n, min: sorted[0], max: sorted[n - 1],
    p25: at(0.25), median: at(0.5), p75: at(0.75), p90: at(0.9), p99: at(0.99) };
}

// histogram puts the counts into buckets up to the 99th percentile; the last
// bucket holds the long tail.
export function histogram(counts, buckets = 24) {
  const s = stats(counts);
  if (!s) return [];
  const top = Math.max(s.p99, 1), width = Math.max(1, Math.ceil(top / buckets));
  const bars = Array.from({ length: Math.ceil(top / width) }, (_, i) => ({ from: i * width, to: (i + 1) * width - 1, count: 0 }));
  for (const v of counts) bars[Math.min(bars.length - 1, Math.floor(v / width))].count++;
  bars.at(-1).to = s.max;
  return bars;
}
