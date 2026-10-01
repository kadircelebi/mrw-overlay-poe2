import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { createItem, setRarity, applyCurrency, currencyReason } from '../public/craft/engine.mjs';
import { applicable, essenceRows, specialReason, applySpecial } from '../public/craft/special.mjs';
import { craftText } from '../public/craft/trade.mjs';
import { allKeys, setLang, t } from '../public/craft/i18n.mjs';

const json = async name => JSON.parse(await readFile(new URL('../public/craft/data/' + name, import.meta.url)));
const { classes } = await json('classes.json');
const rules = await json('currency-rules.source.json');
const special = (await json('special-currencies.json')).rules;
const pages = classes.flatMap(c => c.variants.map(v => ({ cls: c, page: v.page })));
const datasets = Object.fromEntries(await Promise.all(pages.map(async ({ page }) => [page, await json(`${page}.mods.json`)])));
// A fixed sequence keeps the rolls repeatable.
const seq = () => { let i = 0; return () => ((i++ * 0.37) % 1); };

test('every craft language has the same keys', () => {
  const keys = allKeys();
  assert.deepEqual([...keys.tr].sort(), [...keys.en].sort());
  assert.deepEqual([...keys.zh].sort(), [...keys.en].sort());
  setLang('zh-Hant'); assert.equal(t('err.needIlvl', 40), '需要 Item Level 40。');
  setLang('en'); assert.equal(t('err.needIlvl', 40), 'Requires Item Level 40.');
});

test('every class page has real weights and takes basic currency', () => {
  assert.ok(classes.length >= 20);
  for (const { cls, page } of pages) {
    const data = datasets[page];
    const normal = data.mods.filter(m => m.pool === 'normal' && ['Prefix', 'Suffix'].includes(m.affix));
    assert.ok(normal.some(m => m.weight > 1), `${page}: placeholder weights`);
    assert.ok(!data.tags.includes('default'), `${page}: 'default' would let every essence variant through`);
    let item = setRarity(createItem(page), 'Rare');
    for (let i = 0; i < 4; i++) item = applyCurrency(item, data, 'exalted', rules.exalted, seq());
    assert.equal(item.mods.length, 4, page);
    assert.equal(currencyReason(item, data, 'chaos', rules.chaos), '', page);
    assert.match(craftText(item, cls.itemClass), new RegExp(`^Item Class: ${cls.itemClass}\\nRarity: Rare`), page);
  }
});

test('bones and essences follow the item class', () => {
  const bone = { Gloves: 'rib', Rings: 'collarbone', Bows: 'jawbone', Quivers: 'jawbone', Foci: 'rib' };
  for (const [itemClass, kind] of Object.entries(bone)) {
    const cls = classes.find(c => c.itemClass === itemClass);
    const data = datasets[cls.variants[0].page];
    const bones = Object.entries(special).filter(([, r]) => r.operation === 'desecrate' && applicable(r, data)).map(([id]) => id.split('-')[1]);
    assert.deepEqual([...new Set(bones)], [kind], itemClass);
  }
  // Every class but a few has an essence that fits it, and using one works.
  for (const { page } of pages) {
    const data = datasets[page];
    const essences = Object.entries(special).filter(([, r]) => r.operation === 'essence' && !r.removes &&
      applicable(r, data) && essenceRows(data, r).length);
    assert.ok(essences.length, `${page}: no essence`);
    const [, rule] = essences[0];
    const magic = setRarity(createItem(page), 'Magic');
    assert.equal(specialReason(magic, data, rule), '', page);
    const next = applySpecial(magic, data, rule, seq());
    assert.equal(next.rarity, 'Rare', page);
  }
});

test('a defence essence offers only the matching defence variant', () => {
  for (const page of ['Gloves_str', 'Boots_dex', 'Helmets_int', 'Body_Armours_str_int']) {
    const rows = essenceRows(datasets[page], special['greater-essence-of-enhancement']);
    assert.equal(rows.length, 1, page);
  }
});
