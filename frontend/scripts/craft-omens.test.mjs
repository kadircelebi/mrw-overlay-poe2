import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { createItem,setRarity,manualAdd,count } from '../public/craft/engine.mjs';
import { omenDefinitions,relevantOmens,omenEffects,orbOmenReason,applyOrbOmens } from '../public/craft/omens.mjs';
import { applySpecial } from '../public/craft/special.mjs';
import { summarize } from '../public/craft/ledger.mjs';
const json = async name => JSON.parse(await readFile(new URL('../public/craft/data/'+name,import.meta.url)));
const data = await json('Gloves_str.mods.json'), rules = await json('currency-rules.source.json');
const special = (await json('special-currencies.json')).rules, omens = omenDefinitions(rules);
const empty = () => setRarity(createItem(),'Rare'), zero=()=>0;
const effects = (ids,id='exalted',rule=rules.exalted,d=data) => omenEffects(omens,ids,id,rule,d);

test('Exalted side omens constrain every added modifier, including greater/perfect variants',()=>{
  for (const [id,side] of [['omen-of-sinistral-exaltation','Prefix'],['omen-of-dextral-exaltation','Suffix']]) {
    for (const currency of ['exalted','greater-exalted-orb','perfect-exalted-orb']) {
      const rule=rules[currency], e=effects([id],currency,rule);
      const next=applyOrbOmens(empty(),data,currency,rule,e,zero);
      assert.equal(next.mods.length,1); assert.equal(next.mods[0].affix,side);
    }
  }
});
test('Greater Exaltation adds two distinct affixes and combines with a side restriction',()=>{
  const e=effects(['omen-of-greater-exaltation','omen-of-dextral-exaltation']);
  const next=applyOrbOmens(empty(),data,'exalted',rules.exalted,e,zero);
  assert.equal(count(next,'Suffix'),2); assert.equal(count(next,'Prefix'),0);
  assert.notEqual(next.mods[0].families[0],next.mods[1].families[0]);
  assert.equal(empty().mods.length,0);
  const full=applyOrbOmens(next,data,'exalted',rules.exalted,effects(['omen-of-dextral-exaltation']),zero);
  assert.match(orbOmenReason(full,data,'exalted',rules.exalted,e),/boş/);
  assert.match(orbOmenReason(next,data,'exalted',rules.exalted,e),/İki affix/);
});
test('Conflicting and unrelated omens are rejected before mutation',()=>{
  assert.throws(()=>effects(['omen-of-sinistral-exaltation','omen-of-dextral-exaltation']),/Çelişen/);
  assert.throws(()=>effects(['omen-of-sinistral-necromancy']),/kullanılamaz/);
});
test('Necromancy fixes the reserved/revealed side, including a full item',()=>{
  const e=effects(['omen-of-dextral-necromancy'],'preserved-rib',special['preserved-rib']);
  const next=applySpecial(empty(),data,special['preserved-rib'],zero,e);
  assert.equal(next.mods[0].affix,'Suffix');
  assert.ok(next.reveal.choices.every(m=>m.affix==='Suffix'));
  let full=empty();
  for(const side of ['Prefix','Suffix']) for (let i=0;i<3;i++) {
    const row=data.mods.find(m=>m.pool==='normal'&&m.affix===side&&!full.mods.some(x=>x.families[0]===m.families[0]));
    full=manualAdd(full,row,zero);
  }
  const replaced=applySpecial(full,data,special['preserved-rib'],zero,e);
  assert.equal(count(replaced,'Prefix'),3); assert.equal(count(replaced,'Suffix'),3);
  assert.deepEqual(replaced.mods.filter(m=>m.affix==='Prefix'),full.mods.filter(m=>m.affix==='Prefix'));
});
test('Liege is hidden for armour and limits synthetic Jewellery reveal to Amanamu',()=>{
  const bone=special['preserved-rib'];
  assert.ok(!relevantOmens(omens,'preserved-rib',bone,data).some(([id])=>id==='omen-of-the-liege'));
  assert.throws(()=>effects(['omen-of-the-liege'],'preserved-rib',bone),/kullanılamaz/);
  // Same supplied modifier fixture under a synthetic class: this tests filtering, not real ring data.
  const fixture={...data,options:{...data.options,ItemClassesCode:'Ring'}};
  const collar=special['preserved-collarbone'];
  const e=effects(['omen-of-the-liege'],'preserved-collarbone',collar,fixture);
  const next=applySpecial(empty(),fixture,collar,zero,e);
  assert.ok(next.reveal.choices.length>0);
  assert.ok(next.reveal.choices.every(m=>m.pool==='desecrated'&&m.tags.includes('amanamu_mod')));
});
test('One combined operation prices orb and omens separately and supports legacy history',()=>{
  const history=[{usage:{id:'old',name:'Old',quantity:1,unit_ex:2}},
    {usages:[{id:'exalted',name:'Exalted',quantity:1,unit_ex:1},
      {id:'omen',name:'Omen',quantity:1,unit_ex:5}]}];
  assert.equal(summarize(history).total,8); assert.equal(summarize(history).rows.length,3);
  assert.equal(summarize(history.slice(0,1)).total,2);
});
