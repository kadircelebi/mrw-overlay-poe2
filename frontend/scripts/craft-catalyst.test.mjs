import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { createItem, setRarity, applyCurrency, currencyReason, candidates, roll, rolledText } from '../public/craft/engine.mjs';
import { omenDefinitions, omenEffects, orbOmenReason, applyOrbOmens } from '../public/craft/omens.mjs';
import { catalystRules, catalystBoost, augmentedValues, augmentedMod, catalystFromCopy, catalystMultiplier } from '../public/craft/catalyst.mjs';
import { setLang } from '../public/craft/i18n.mjs';
setLang('tr');
const json = async name => JSON.parse(await readFile(new URL('../public/craft/data/' + name, import.meta.url)));
const ring = await json('Rings.mods.json'), gloves = await json('Gloves_str.mods.json');
const amulet = await json('Amulets.mods.json');
const rules = { ...(await json('currency-rules.source.json')), ...catalystRules() };
const omens = omenDefinitions(rules);
const rareRing = () => ({ ...setRarity(createItem('Rings'), 'Rare'), ilvl: 82 });

test('a catalyst adds 1% of its type up to 20%, only on rings and amulets, and replaces another type', () => {
  const flesh = rules['flesh-catalyst'];
  let item = rareRing();
  for (let i = 0; i < 20; i++) item = applyCurrency(item, ring, 'flesh-catalyst', flesh);
  assert.deepEqual(item.catalyst, { id: 'flesh-catalyst', quality: 20 });
  assert.match(currencyReason(item, ring, 'flesh-catalyst', flesh), /%20/);
  item = applyCurrency(item, ring, 'reaver-catalyst', rules['reaver-catalyst']);
  assert.equal(item.catalyst.id, 'reaver-catalyst');
  assert.match(currencyReason(setRarity(createItem(), 'Rare'), gloves, 'flesh-catalyst', flesh), /yüzük/);
});

test('quality raises matching values as the game shows them (rounded down)', () => {
  const life = { tags: ['resource', 'life'], values: [100] }, regen = { tags: ['life'], values: [6] };
  const at = q => ({ catalyst: { id: 'flesh-catalyst', quality: q } });
  assert.deepEqual(augmentedValues(life, at(1)), [101]);
  assert.deepEqual(augmentedValues(life, at(20)), [120]);
  assert.deepEqual(augmentedValues(regen, at(5)), [6]);
  assert.deepEqual(augmentedValues(regen, at(20)), [7]);
  assert.equal(augmentedValues({ tags: ['fire'], values: [10] }, at(20)), null);
});

test('fixed spell skill levels rise at 34% caster quality without changing the saved tier', () => {
  const spell = roll(amulet.mods.find(m => m.pool === 'normal' && m.text === '+3 to Level of all Spell Skills'));
  const original = structuredClone(spell);
  for (const [quality, expected] of [[20, 3], [33, 3], [34, 4], [40, 4]]) {
    const raised = augmentedMod(spell, { catalyst: { id: 'sibilant-catalyst', quality } });
    assert.equal(rolledText(raised), `+${expected} to Level of all Spell Skills`);
    assert.deepEqual(spell, original);
  }
  assert.equal(augmentedMod(spell, { catalyst: { id: 'neural-catalyst', quality: 40 } }), null);
  assert.equal(augmentedMod(spell, { catalyst: { id: 'sibilant-catalyst', quality: 0 } }), null);
  assert.equal(rolledText(spell), '+3 to Level of all Spell Skills');
});

test('display augmentation preserves ranged caster rolls and fixed reference numbers', () => {
  const item = { catalyst: { id: 'sibilant-catalyst', quality: 40 } };
  const speed = roll(amulet.mods.find(m => m.pool === 'normal' && m.tier === 1 && m.tags.includes('caster_speed')), () => 0);
  assert.equal(rolledText(augmentedMod(speed, item)), '35% increased Cast Speed');
  const reference = { text: '20% chance for Skills to retain 40% of Glory on use', tags: ['caster'], values: [] };
  assert.equal(augmentedMod(reference, item), null);
});

test('the omen boosts matching weights by the quality and uses the quality up', () => {
  const exalted = rules.exalted, effects = omenEffects(omens, ['omen-of-catalysing-exaltation'], 'exalted', exalted, ring);
  assert.equal(effects.catalyse, true);
  assert.match(orbOmenReason(rareRing(), ring, 'exalted', exalted, effects), /kalite/);
  const item = { ...rareRing(), catalyst: { id: 'flesh-catalyst', quality: 20 } };
  const rows = candidates(item, ring, {});
  const boosted = catalystBoost(item, rows);
  const lifeRow = rows.findIndex(r => r.tags.includes('life'));
  assert.equal(boosted[lifeRow].weight, rows[lifeRow].weight * catalystMultiplier(20));
  assert.equal(catalystMultiplier(20), 5);
  const next = applyOrbOmens(item, ring, 'exalted', exalted, effects, () => 0.5);
  assert.equal(next.mods.length, 1);
  assert.equal(next.catalyst.quality, 0);
  // At 20% life modifiers are about half of what an Exalted Orb adds.
  let life = 0;
  for (let i = 0; i < 400; i++) if (applyOrbOmens(item, ring, 'exalted', exalted, effects, () => (i + 0.5) / 400).mods[0].tags.includes('life')) life++;
  assert.ok(life > 120 && life < 260, `life ${life}/400`);
});

test('a copied ring brings its catalyst quality along', () => {
  assert.deepEqual(catalystFromCopy('Item Class: Rings\nQuality (Life Modifiers): +20% (augmented)\n'), { id: 'flesh-catalyst', quality: 20 });
  assert.equal(catalystFromCopy('Quality: +20% (augmented)'), null);
});
