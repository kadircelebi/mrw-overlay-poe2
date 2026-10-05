import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { createItem, setRarity, manualAdd } from '../public/craft/engine.mjs';
import { omenDefinitions, omenEffects, applyOrbOmens } from '../public/craft/omens.mjs';
import { applySpecial, startReveal } from '../public/craft/special.mjs';
import { orbOdds, offerOdds, desecrateOdds, revealOdds } from '../public/craft/odds.mjs';

const json = async name => JSON.parse(await readFile(new URL('../public/craft/data/' + name, import.meta.url)));
const amulets = await json('Amulets.mods.json'), rules = await json('currency-rules.source.json');
const special = (await json('special-currencies.json')).rules, omens = omenDefinitions(rules);
const effects = (ids, id) => omenEffects(omens, ids, id, rules[id], amulets);
const sum = map => [...map.values()].reduce((a, b) => a + b, 0);
const castSpeedT1 = amulets.mods.find(m => m.pool === 'normal' && m.families[0] === 'IncreasedCastSpeed' && m.tier === 1);
const castSpeedT6 = amulets.mods.find(m => m.pool === 'normal' && m.families[0] === 'IncreasedCastSpeed' && m.tier === 6);
// A seeded generator so the sampled runs are repeatable.
const seeded = (seed = 1) => () => ((seed = (seed * 1664525 + 1013904223) >>> 0) / 2 ** 32);
const rare = () => ({ ...setRarity(createItem('Amulets'), 'Rare'), ilvl: 82 });
const row = (family, tier) => amulets.mods.find(m => m.pool === 'normal' && m.families[0] === family && m.tier === tier);
const partial = () => [row('IncreasedLife', 1), row('FireResistance', 1)].reduce((item, r) => manualAdd(item, r, () => 0), rare());
const full = () => [row('IncreasedLife', 1), row('IncreasedMana', 1), row('IncreasedEnergyShield', 1),
  row('FireResistance', 1), row('ColdResistance', 1), row('LightningResistance', 1)].reduce((item, r) => manualAdd(item, r, () => 0), rare());

// Counts how often a sampled outcome holds each family.
function sample(times, run) {
  const seen = new Map(), random = seeded(7);
  for (let i = 0; i < times; i++) for (const id of run(random)) seen.set(id, (seen.get(id) || 0) + 1 / times);
  return seen;
}
const familyOf = (map, family) => amulets.mods.filter(m => m.families[0] === family).reduce((p, m) => p + (map.get(m.source_id) || 0), 0);
const close = (a, b, tolerance) => assert.ok(Math.abs(a - b) < tolerance, `${a} vs ${b}`);

test('One Exalted Orb: the odds add up to 1 and Perfect drops modifiers below level 50', () => {
  const plain = orbOdds(rare(), amulets, 'exalted', rules.exalted);
  close(sum(plain), 1, 1e-9);
  assert.ok(plain.get(castSpeedT6.source_id) > 0);
  const perfect = orbOdds(rare(), amulets, 'perfect-exalted-orb', rules['perfect-exalted-orb']);
  close(sum(perfect), 1, 1e-9);
  assert.equal(perfect.get(castSpeedT6.source_id), undefined);
  assert.ok(perfect.get(castSpeedT1.source_id) > plain.get(castSpeedT1.source_id));
});

test('Side omen and Greater Exaltation match the engine', () => {
  const e = effects(['omen-of-greater-exaltation', 'omen-of-dextral-exaltation'], 'exalted');
  const odds = orbOdds(rare(), amulets, 'exalted', rules.exalted, e);
  close(sum(odds), 2, 1e-9);
  assert.ok([...odds.keys()].every(id => amulets.mods.find(m => m.source_id === id).affix === 'Suffix'));
  // The engine checks every pair before rolling, so fewer samples here.
  const seen = sample(3000, random => applyOrbOmens(rare(), amulets, 'exalted', rules.exalted, e, random).mods.map(m => m.source_id));
  for (const family of ['IncreasedCastSpeed', 'FireResistance', 'ItemFoundRarityIncrease']) close(familyOf(odds, family), familyOf(seen, family), 0.03);
});

test('Chaos Orb averages over the removed modifier', () => {
  const item = full(), odds = orbOdds(item, amulets, 'chaos', rules.chaos);
  close(sum(odds), 1, 1e-9);
  const seen = sample(20000, random => {
    const next = applyOrbOmens(item, amulets, 'chaos', rules.chaos, effects([], 'chaos'), random);
    return next.mods.filter(m => !item.mods.some(old => old.source_id === m.source_id)).map(m => m.source_id);
  });
  for (const family of ['IncreasedCastSpeed', 'IncreasedLife', 'FireResistance', 'Strength']) close(familyOf(odds, family), familyOf(seen, family), 0.02);
});

test('Three options without repeating a family', () => {
  const rows = [{ source_id: 'a', affix: 'Suffix', families: ['A'], weight: 1 }, { source_id: 'b', affix: 'Suffix', families: ['B'], weight: 1 },
    { source_id: 'c', affix: 'Suffix', families: ['C'], weight: 2 }, { source_id: 'd', affix: 'Suffix', families: ['D'], weight: 4 }];
  const odds = offerOdds(rows);
  close(sum(odds), 3, 1e-9);
  // One pick: d holds half of the weight.
  close(offerOdds(rows, 1).get('d'), 0.5, 1e-12);
  assert.ok(odds.get('d') > odds.get('c') && odds.get('c') > odds.get('a'));
});

test('Bone offers match the Well of Souls draws', () => {
  const rule = special['preserved-collarbone'], item = partial(), e = { side: null, tags: [], quantity: 1 };
  const odds = desecrateOdds(item, amulets, rule, e);
  close(sum(odds), 3, 1e-6);
  const seen = sample(6000, random => {
    const next = startReveal(applySpecial(item, amulets, rule, random, e), amulets, {}, random);
    return next.reveal.choices.map(m => m.source_id);
  });
  for (const family of ['IncreasedCastSpeed', 'Strength', 'IncreasedMana']) close(familyOf(odds, family), familyOf(seen, family), 0.03);
  const unrevealed = applySpecial(item, amulets, rule, seeded(3), e);
  close(sum(revealOdds(unrevealed, amulets)), 3, 1e-6);
});
