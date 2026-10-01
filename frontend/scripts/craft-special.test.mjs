import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { createItem,setRarity,manualAdd,applyCurrency,currencyReason,clearMods,overlaps } from '../public/craft/engine.mjs';
import { applicable,essenceRows,specialReason,applySpecial,revealChoice } from '../public/craft/special.mjs';
import { usageEntry,summarize } from '../public/craft/ledger.mjs';
import { setLang } from '../public/craft/i18n.mjs';
// The assertions below match the Turkish messages.
setLang('tr');
const json = async name => JSON.parse(await readFile(new URL('../public/craft/data/'+name,import.meta.url)));
const data = await json('Gloves_str.mods.json');
const special = (await json('special-currencies.json')).rules;
const rules = await json('currency-rules.source.json');
const zero = () => 0;

test('Bones are filtered by item class; corrected Gnawed and Ancient restrictions',()=>{
  const visible = Object.entries(special).filter(([,r])=>r.operation==='desecrate' && applicable(r,data)).map(([id])=>id);
  assert.deepEqual(visible.sort(),['ancient-rib','gnawed-rib','preserved-rib']);
  for (const [className, bone] of [['Bow','jawbone'],['Ring','collarbone'],['Jewel','cranium']]) {
    const d = {...data,options:{ItemClassesCode:className}};
    assert.equal(applicable(special['preserved-'+bone],d),true);
    assert.equal(applicable(special['preserved-rib'],d),false);
  }
  assert.equal(special['ancient-rib'].minimum,40);
  assert.equal(special['preserved-rib'].minimum,undefined);
  assert.match(specialReason(setRarity(createItem(),'Rare'),data,special['gnawed-rib']),/64/);
});

test('Essence upgrades Magic with guaranteed mod while preserving existing affixes',()=>{
  const rule = special['greater-essence-of-the-mind'];
  assert.match(specialReason(createItem(),data,rule),/Magic/);
  const suffix = data.mods.find(m=>m.pool==='normal'&&m.affix==='Suffix');
  const item = manualAdd(createItem(),suffix,zero);
  const next = applySpecial(item,data,rule,zero);
  assert.equal(next.rarity,'Rare'); assert.equal(next.mods.length,2);
  assert.deepEqual(next.mods[0],item.mods[0]);
  assert.equal(next.mods[1].name,rule.name);
  assert.equal(next.mods[1].values[0],80);
  assert.match(specialReason(setRarity(item,'Rare'),data,rule),/Magic/);
});

test('Armour essence filters source variants to selected defence category',()=>{
  const rows = essenceRows(data,special['greater-essence-of-enhancement']);
  assert.equal(rows.length,1); assert.ok(rows[0].source_id.includes('PhysicalDamageReductionRating'));
});

test('Special essence replaces a random mod, retaining rarity and total count',()=>{
  const item = manualAdd(setRarity(createItem(),'Rare'),data.mods.find(m=>m.pool==='normal'&&m.affix==='Prefix'),zero);
  const next = applySpecial(item,data,special['essence-of-hysteria'],zero);
  assert.equal(next.mods.length,1); assert.equal(next.rarity,'Rare');
  assert.equal(next.mods[0].name,'Essence of Hysteria');
  assert.deepEqual(item.mods[0].pool,'normal');
  assert.match(specialReason(setRarity(createItem(),'Rare'),data,special['essence-of-hysteria']),/Çıkarılacak/);
});

test('Desecrate reserves one slot; reveal yields distinct families and charges no extra currency',()=>{
  const item = setRarity(createItem(),'Rare');
  const next = applySpecial(item,data,special['preserved-rib'],zero);
  assert.equal(next.mods.length,1); assert.equal(next.reveal.choices.length,3);
  assert.ok(next.reveal.choices.every(m=>m.affix===next.mods[0].affix));
  for (let i=0;i<3;i++) for(let j=i+1;j<3;j++) assert.equal(overlaps(next.reveal.choices[i],next.reveal.choices[j]),false);
  assert.match(currencyReason(next,data,'exalted',rules.exalted),/bekleyen/);
  const chosen = revealChoice(next,1);
  assert.equal(chosen.reveal,undefined); assert.equal(chosen.mods[0].desecrated,true);
  assert.equal(chosen.mods[0].source_id,next.reveal.choices[1].source_id);
  assert.match(specialReason(chosen,data,special['preserved-rib']),/zaten/);
});

test('Desecrate on full item replaces one mod and preserves 3/3 slots',()=>{
  let item = setRarity(createItem(),'Rare');
  for(let i=0;i<6;i++) item=applyCurrency(item,data,'exalted',rules.exalted,zero);
  const next = applySpecial(item,data,special['preserved-rib'],zero);
  assert.equal(next.mods.length,6);
  assert.equal(next.mods.filter(m=>m.affix==='Prefix').length,3);
  assert.equal(revealChoice(next,0).mods.length,6);
});

test('Ledger snapshots prices at use and retains costs after affix removal; unknown is not zero',()=>{
  const prices = {currency:[{api_id:'preserved-rib',name:'Preserved Rib',value_ex:2}],league:'Test',generated_at:'2026-10-01',origin:'test'};
  const first = usageEntry('preserved-rib','Preserved Rib',prices);
  prices.currency[0].value_ex=9;
  const history = [{usage:first},{label:'affix removed'},{usage:usageEntry('preserved-rib','Preserved Rib',prices)},{usage:usageEntry('missing','Unknown',prices)}];
  const summary=summarize(history);
  assert.equal(summary.total,11); assert.equal(summary.unknown,1); assert.equal(summary.rows[0].quantity,2);
  assert.equal(summarize(history.slice(0,2)).total,2);
});
