import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { createItem, setRarity, manualAdd, candidates, applyCurrency, currencyReason, rollPools } from '../public/craft/engine.mjs';
import { omenDefinitions, omenEffects, orbOmenReason, applyOrbOmens } from '../public/craft/omens.mjs';
import { runesFor, runeStatLines } from '../public/craft/import.mjs';
import { craftText } from '../public/craft/trade.mjs';
import { setLang } from '../public/craft/i18n.mjs';
// The assertions below match the Turkish messages.
setLang('tr');
const json = async name => JSON.parse(await readFile(new URL('../public/craft/data/' + name, import.meta.url)));
const data = await json('Gloves_int.mods.json'), rules = await json('currency-rules.source.json');
const runes = (await json('special-currencies.json')).runes, omens = omenDefinitions(rules);
const row = (side, pred) => data.mods.find(m => m.pool === 'normal' && m.affix === side && pred(m));
const prefixes = [...new Set(data.mods.filter(m => m.pool === 'normal' && m.affix === 'Prefix').map(m => m.families[0]))];
const suffixes = [...new Set(data.mods.filter(m => m.pool === 'normal' && m.affix === 'Suffix').map(m => m.families[0]))];
const topOf = family => data.mods.filter(m => m.pool === 'normal' && m.families[0] === family).sort((a, b) => a.tier - b.tier)[0];
// A Rare with three prefixes and three suffixes from different families.
function full() {
  let item = { ...setRarity(createItem('Gloves_int'), 'Rare'), ilvl: 82 };
  for (const family of [...prefixes.slice(0, 3), ...suffixes.slice(0, 3)]) item = manualAdd(item, topOf(family), () => 0);
  return item;
}
const effects = (ids, id) => omenEffects(omens, ids, id, rules[id], data);

test('Fracturing needs four affixes, locks one, and nothing removes or rerolls it', () => {
  const three = { ...setRarity(createItem('Gloves_int'), 'Rare'), mods: full().mods.slice(0, 3) };
  assert.match(currencyReason(three, data, 'fracturing-orb', rules['fracturing-orb']), /En az 4/);
  // random() = .99 picks the last affix.
  const item = applyCurrency(full(), data, 'fracturing-orb', rules['fracturing-orb'], () => 0.99);
  assert.equal(item.mods.filter(m => m.fractured).length, 1);
  assert.ok(item.mods[5].fractured);
  assert.match(currencyReason(item, data, 'fracturing-orb', rules['fracturing-orb']), /zaten kilitli/);
  const locked = item.mods[5];
  for (const r of [0, 0.5, 0.99]) {
    const annulled = applyCurrency(item, data, 'annu', rules.annu, () => r);
    assert.ok(annulled.mods.some(m => m.fractured && m.source_id === locked.source_id));
    const divined = applyCurrency(item, data, 'divine', rules.divine, () => r);
    assert.deepEqual(divined.mods.find(m => m.fractured).values, locked.values);
  }
  const onlyLocked = { ...item, mods: [locked] };
  assert.match(currencyReason(onlyLocked, data, 'annu', rules.annu), /kilitli/);
});

test('Omen of Light makes Annulment remove only a Desecrated affix', () => {
  const item = full();
  assert.match(orbOmenReason(item, data, 'annu', rules.annu, effects(['omen-of-light'], 'annu')), /Desecrated affix yok/);
  const marked = { ...item, mods: item.mods.map((m, i) => i === 4 ? { ...m, desecrated: true } : m) };
  const next = applyOrbOmens(marked, data, 'annu', rules.annu, effects(['omen-of-light'], 'annu'), () => 0);
  assert.equal(next.mods.length, 5);
  assert.ok(!next.mods.some(m => m.desecrated));
});

test('Annulment and Chaos side, greater and whittling omens', () => {
  const item = full();
  const sinistral = applyOrbOmens(item, data, 'annu', rules.annu, effects(['omen-of-sinistral-annulment'], 'annu'), () => 0.99);
  assert.equal(sinistral.mods.filter(m => m.affix === 'Prefix').length, 2);
  const greater = applyOrbOmens(item, data, 'annu', rules.annu, effects(['omen-of-greater-annulment'], 'annu'), () => 0);
  assert.equal(greater.mods.length, 4);
  // Erasure: the Chaos Orb removes a suffix; the new affix may go anywhere.
  const erased = applyOrbOmens(item, data, 'chaos', rules.chaos, effects(['omen-of-dextral-erasure'], 'chaos'), () => 0);
  assert.equal(erased.mods.length, 6);
  assert.ok(item.mods.filter(m => m.affix === 'Prefix').every(p => erased.mods.some(m => m.source_id === p.source_id)));
  // Whittling: the lowest-level affix is the one removed.
  const lowest = Math.min(...item.mods.map(m => m.required_ilvl));
  const whittled = applyOrbOmens(item, data, 'annu', rules.annu, { side: null, tags: [], quantity: 1, removal: { lowest: true } }, () => 0);
  assert.equal(whittled.mods.filter(m => m.required_ilvl === lowest).length, item.mods.filter(m => m.required_ilvl === lowest).length - 1);
});

test('Desecrate, then Fracture before revealing: the three other affixes share the chance', async () => {
  const special = (await json('special-currencies.json')).rules;
  const { applySpecial, startReveal, rerollReveal, revealChoice } = await import('../public/craft/special.mjs');
  const { fracturable, removable } = await import('../public/craft/engine.mjs');
  let item = { ...setRarity(createItem('Gloves_int'), 'Rare'), ilvl: 82 };
  for (const family of [...prefixes.slice(0, 2), suffixes[0]]) item = manualAdd(item, topOf(family), () => 0);
  item = applySpecial(item, data, special['preserved-rib'], () => 0);
  assert.equal(item.mods.length, 4);
  assert.ok(item.mods[3].unrevealed && !item.reveal);
  // Crafting goes on with the modifier unrevealed.
  assert.equal(currencyReason(item, data, 'fracturing-orb', rules['fracturing-orb']), '');
  assert.equal(fracturable(item).length, 3);
  for (const r of [0, 0.5, 0.99]) {
    const fractured = applyCurrency(item, data, 'fracturing-orb', rules['fracturing-orb'], () => r);
    assert.ok(!fractured.mods[3].fractured);
    assert.equal(fractured.mods.filter(m => m.fractured).length, 1);
  }
  // Chaos and Annulment can take it; Omen of Light only it; Whittling never.
  assert.ok(removable(item).includes(3));
  assert.deepEqual(removable(item, { desecrated: true }), [3]);
  assert.ok(!removable(item, { lowest: true }).includes(3));
  // Revealing with Omen of Abyssal Echoes allows one reroll of the options.
  const plain = startReveal(item, data, {}, () => 0);
  assert.throws(() => rerollReveal(plain, data), /Abyssal Echoes/);
  const echoed = startReveal(item, data, { echo: true }, () => 0);
  assert.equal(echoed.reveal.choices.length, 3);
  const rerolled = rerollReveal(echoed, data, () => 0.7);
  assert.equal(rerolled.reveal.choices.length, 3);
  assert.ok(rerolled.reveal.choices.every(m => m.affix === item.mods[3].affix));
  assert.throws(() => rerollReveal(rerolled, data), /Abyssal Echoes/);
  const done = revealChoice(rerolled, 0);
  assert.ok(done.mods[3].desecrated && !done.mods[3].unrevealed && !done.reveal);
});

test('A socketed rune adds its pool, with measured weights, to random rolls', () => {
  assert.deepEqual(runes['kolrs-hunt'].pages.sort(), ['Gloves_dex', 'Gloves_dex_int', 'Gloves_int', 'Gloves_str', 'Gloves_str_dex', 'Gloves_str_int']);
  const plus2 = data.mods.find(m => m.source_id === 'MarksmanInfluenceProjectileSkills2');
  assert.equal(plus2.weight, 500);
  const item = { ...setRarity(createItem('Gloves_int'), 'Rare'), ilvl: 82 };
  assert.ok(!candidates(item, data).some(m => m.pool === 'marksman'));
  const socketed = { ...item, rune: 'kolrs-hunt', runePool: 'marksman' };
  assert.deepEqual(rollPools(socketed), ['normal', 'marksman']);
  const pool = candidates(socketed, data);
  assert.ok(pool.some(m => m.source_id === 'MarksmanInfluenceProjectileSkills2'));
  assert.ok(pool.some(m => m.pool === 'normal'));
});

test('A copied item brings its rune and fractured affix', () => {
  const raw = 'Item Class: Gloves\nRarity: Rare\nHate Palm\nSirenscale Gloves\n--------\n20% increased Armour, Evasion and Energy Shield (rune)\nCan roll Marksman modifiers (rune)\n';
  assert.deepEqual(runesFor(raw, 'Gloves_int', runes), ['kolrs-hunt']);
  assert.deepEqual(runeStatLines(raw, runes), ['20% increased Armour, Evasion and Energy Shield']);
  assert.deepEqual(runesFor(raw, 'Boots_int', runes), []);
  assert.deepEqual(runesFor(raw + '+1 Suffix Modifier allowed (rune)\n', 'Boots_int', runes), ['serles-triumph']);
  const item = { rarity: 'Rare', ilvl: 81, mods: [{ ...row('Prefix', () => true), values: [10], fractured: true }] };
  assert.match(craftText(item, 'Gloves'), /\{ Fractured Prefix Modifier/);
});

test('Only Greater, Perfect and the corrupted essences are offered', async () => {
  const special = (await json('special-currencies.json')).rules;
  const { essenceTier } = await import('../public/craft/special.mjs');
  assert.equal(essenceTier('lesser-essence-of-the-mind', special['lesser-essence-of-the-mind']), null);
  assert.equal(essenceTier('essence-of-the-mind', special['essence-of-the-mind']), null);
  assert.equal(essenceTier('greater-essence-of-the-mind', special['greater-essence-of-the-mind']), 'greater');
  assert.equal(essenceTier('perfect-essence-of-the-body', special['perfect-essence-of-the-body']), 'perfect');
  assert.equal(essenceTier('essence-of-hysteria', special['essence-of-hysteria']), 'special');
  // Greater turns a Magic item Rare; Perfect replaces an affix on a Rare.
  assert.deepEqual(special['greater-essence-of-the-mind'].beforeRarity, ['Magic']);
  assert.equal(special['perfect-essence-of-the-body'].removes, true);
});

test("Serle's Triumph allows a fourth suffix; Astrid's Creativity a second crafted modifier", async () => {
  const special = (await json('special-currencies.json')).rules;
  const { applySpecial, specialReason } = await import('../public/craft/special.mjs');
  const { sideLimit, isFull } = await import('../public/craft/engine.mjs');
  const item = full();
  assert.ok(isFull(item));
  const serle = { ...item, suffixBonus: 1 };
  assert.equal(sideLimit(serle, 'Suffix'), 4);
  assert.equal(sideLimit(serle, 'Prefix'), 3);
  assert.ok(!isFull(serle));
  const next = applyCurrency(serle, data, 'exalted', rules.exalted, () => 0.99);
  assert.equal(next.mods.length, 7);
  // One crafted (essence) modifier; Perfect essences need a free crafted slot.
  const five = { ...item, mods: item.mods.slice(0, 5) };
  const perfect = Object.entries(special).find(([id, r]) => id.startsWith('perfect-') && specialReason(five, data, r) === '');
  assert.ok(perfect, 'a Perfect essence fits the test gloves');
  const once = applySpecial(five, data, perfect[1], () => 0);
  assert.match(specialReason(once, data, perfect[1]), /crafted/);
  assert.equal(specialReason({ ...once, craftedLimit: 2 }, data, perfect[1]).includes('crafted'), false);
});

test('Crystallisation omens pick the side a Perfect essence removes from', async () => {
  const special = (await json('special-currencies.json')).rules;
  const { applySpecial, specialReason } = await import('../public/craft/special.mjs');
  // Three prefixes and two suffixes: a suffix essence fits whatever is removed.
  const item = { ...full(), mods: full().mods.slice(0, 5) };
  const [id, perfect] = Object.entries(special).find(([key, r]) => key.startsWith('perfect-') && specialReason(item, data, r) === '');
  for (const [omen, side] of [['omen-of-sinistral-crystallisation', 'Prefix'], ['omen-of-dextral-crystallisation', 'Suffix']]) {
    const e = omenEffects(omens, [omen], id, perfect, data);
    assert.equal(e.removal.side, side);
    for (const r of [0, 0.5, 0.99]) {
      const next = applySpecial(item, data, perfect, () => r, e);
      const kept = item.mods.filter(m => m.affix !== side);
      assert.ok(kept.every(k => next.mods.some(m => m.source_id === k.source_id)), `${omen} removed from the other side`);
    }
  }
  assert.throws(() => omenEffects(omens, ['omen-of-sinistral-crystallisation', 'omen-of-dextral-crystallisation'], id, perfect, data));
});
