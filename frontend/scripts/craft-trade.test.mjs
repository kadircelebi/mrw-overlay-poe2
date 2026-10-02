import { test } from 'node:test';
import assert from 'node:assert/strict';
import { craftText } from '../public/craft/trade.mjs';
import { setLang } from '../public/craft/i18n.mjs';
// The assertions below match the Turkish messages.
setLang('tr');

test('market text uses roll values, prefixes first, and a separate desecrated header', () => {
  const text = craftText({rarity:'Rare',ilvl:81,mods:[
    {affix:'Suffix',tier:2,pool:'desecrated',text:'+(13—17)% to Fire and Chaos Resistances',values:[15]},
    {affix:'Prefix',tier:3,pool:'normal',text:'+(80—89) to maximum Mana',values:[85]},
    {affix:'Prefix',tier:1,pool:'normal',text:'(39—42)% increased Armour\n+(42—49) to maximum Life',values:[40,48]},
  ]});
  assert.match(text,/\+85 to maximum Mana/);
  assert.match(text,/40% increased Armour\n\+48 to maximum Life/);
  assert.match(text,/Desecrated Suffix Modifier \(Tier: 2\)/);
  assert.ok(text.indexOf('+85') < text.indexOf('+15%'));
  assert.ok(!text.includes('—'));
});
test('pending reveal cannot be sent to price search', () => {
  assert.throws(() => craftText({rarity:'Rare',ilvl:81,mods:[],reveal:{}}), /Desecrate/);
});
test('crafted and fractured affixes carry their header for the trade search', () => {
  const text = craftText({rarity:'Rare',ilvl:81,mods:[
    {affix:'Suffix',tier:1,pool:'essence',text:'(9—12)% increased Cast Speed',values:[11]},
    {affix:'Prefix',tier:1,pool:'normal',text:'(92—100)% increased Energy Shield',values:[99],fractured:true},
  ]});
  assert.match(text, /\{ Crafted Suffix Modifier \(Tier: 1\) \}\n11% increased Cast Speed/);
  assert.match(text, /\{ Fractured Prefix Modifier \(Tier: 1\) \}\n99% increased Energy Shield/);
});
