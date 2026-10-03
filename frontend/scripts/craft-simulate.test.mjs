import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { createItem, setRarity, manualAdd } from '../public/craft/engine.mjs';
import { omenDefinitions, omenEffects } from '../public/craft/omens.mjs';
import { lineKey, targetOf, isHit, targetReason, runOnce, fastRunner, simData, stats, histogram } from '../public/craft/simulate.mjs';
import { setLang } from '../public/craft/i18n.mjs';
setLang('en');
const json = async name => JSON.parse(await readFile(new URL('../public/craft/data/' + name, import.meta.url)));
const wands = await json('Wands.mods.json'), rules = await json('currency-rules.source.json'), omens = omenDefinitions(rules);
const row = text => wands.mods.find(m => m.pool === 'normal' && m.text === text);
const effects = (ids = [], id = 'chaos') => omenEffects(omens, ids, id, rules[id], wands);
// A small deterministic generator (mulberry32), so the comparison does not
// flake. A plain LCG in floating point is too poor: its correlated draws bias
// the fast runner, which takes several per press, differently from the engine.
const seeded = seed => () => {
  seed = seed + 0x6D2B79F5 | 0;
  let t = Math.imul(seed ^ seed >>> 15, 1 | seed);
  t = t + Math.imul(t ^ t >>> 7, 61 | t) ^ t;
  return ((t ^ t >>> 14) >>> 0) / 4294967296;
};
const rare = mods => mods.reduce((item, text) => manualAdd(item, row(text), () => 0), setRarity({ ...createItem('Wands'), ilvl: 82 }, 'Rare'));

test('a target is a modifier line: all Spell and Fire Spell share a family but not a line', () => {
  const allSpell = targetOf(row('+4 to Level of all Spell Skills'));
  assert.equal(allSpell.family, row('+5 to Level of all Fire Spell Skills').families[0]);
  assert.notEqual(lineKey(row('+5 to Level of all Fire Spell Skills')), allSpell.key);
  // Fire Spell's T1 is not "all Spell T2 or better"; all Spell's own T2 is, its T3 is not.
  assert.equal(isHit(row('+5 to Level of all Fire Spell Skills'), allSpell), false);
  assert.equal(isHit(row('+4 to Level of all Spell Skills'), allSpell), true);
  assert.equal(isHit(row('+3 to Level of all Spell Skills'), allSpell), false);
  // Ranged lines compare without their numbers.
  assert.equal(lineKey(row('+(17—20) to Intelligence')), lineKey(row('+(5—8) to Intelligence')));
});

test('the simulation explains what stops it before it starts', () => {
  const target = targetOf(row('+4 to Level of all Spell Skills'));
  const item = rare(['+(17—20) to Intelligence']);
  assert.equal(targetReason(item, wands, 'chaos', rules.chaos, effects(), target), '');
  assert.match(targetReason(item, wands, 'exalted', rules.exalted, effects([], 'exalted'), target), /Chaos/);
  assert.match(targetReason(item, wands, 'chaos', rules.chaos, effects(), null), /target/);
  assert.match(targetReason({ ...item, ilvl: 60 }, wands, 'chaos', rules.chaos, effects(), target), /cannot roll/);
  const done = rare(['+4 to Level of all Spell Skills']);
  assert.match(targetReason(done, wands, 'chaos', rules.chaos, effects(), target), /already/);
  const fractured = rare(['+1 to Level of all Fire Spell Skills']);
  fractured.mods[0] = { ...fractured.mods[0], fractured: true };
  assert.match(targetReason(fractured, wands, 'chaos', rules.chaos, effects(), target), /fractured/i);
});

test('a run stops on the first press that brings the target', () => {
  const target = targetOf(row('+(17—20) to Intelligence'));
  const item = rare(['+(5—8) to Intelligence', '+1 to Level of all Fire Spell Skills']);
  const result = runOnce(item, simData(item, wands), 'chaos', rules.chaos, effects(), target, { random: seeded(3) });
  assert.ok(result.orbs >= 1);
  assert.ok(result.item.mods.some(m => isHit(m, target)));
  assert.equal(result.item.mods.filter(m => isHit(m, target)).length, 1);
});

// The fast runner must follow the engine's rules: over many runs both give
// the same average number of orbs (within a few standard errors).
for (const [name, text, id, ids, runs] of [
  ['plain Chaos', '+(17—20) to Intelligence', 'chaos', [], 1500],
  ['Chaos with Whittling', '+(17—20) to Intelligence', 'chaos', ['omen-of-whittling'], 1500],
  ['Chaos with Dextral Erasure', '+(17—20) to Intelligence', 'chaos', ['omen-of-dextral-erasure'], 1500],
  ['Perfect Chaos', '+3 to Level of all Spell Skills', 'perfect-chaos-orb', [], 300],
]) {
  test(`fast runner matches the engine: ${name}`, () => {
    const item = rare(['+(5—8) to Intelligence', '+1 to Level of all Fire Spell Skills', row('+(28—30)% increased Cast Speed') ? '+(28—30)% increased Cast Speed' : '+(17—20) to Maximum Mana']
      .filter(text => row(text)));
    const target = targetOf(row(text)), e = effects(ids, id), sd = simData(item, wands);
    const slowRandom = seeded(11), engine = [];
    for (let i = 0; i < runs; i++) engine.push(runOnce(item, sd, id, rules[id], e, target, { random: slowRandom, cap: 3000 }).orbs);
    const fast = fastRunner(item, wands, rules[id], e, target), fastRandom = seeded(29), quick = [];
    for (let i = 0; i < 20000; i++) quick.push(fast(fastRandom, 3000).orbs);
    const mean = a => a.reduce((s, v) => s + v, 0) / a.length;
    const se = a => Math.sqrt(a.reduce((s, v) => s + (v - mean(a)) ** 2, 0) / a.length / a.length);
    const gap = Math.abs(mean(engine) - mean(quick)), limit = 4 * Math.hypot(se(engine), se(quick));
    assert.ok(gap < limit, `engine ${mean(engine).toFixed(1)} vs fast ${mean(quick).toFixed(1)} (limit ${limit.toFixed(1)})`);
  });
}

test('stats and histogram describe the counts', () => {
  const counts = [1, 2, 2, 3, 4, 5, 6, 8, 10, 100];
  const s = stats(counts);
  assert.equal(s.runs, 10); assert.equal(s.min, 1); assert.equal(s.max, 100);
  assert.equal(s.median, 4); assert.equal(s.p90, 10); assert.equal(s.mean, 14.1);
  const bars = histogram(counts, 5);
  assert.equal(bars.reduce((sum, b) => sum + b.count, 0), counts.length);
  assert.equal(bars.at(-1).to, 100);
  assert.equal(stats([]), null);
});
