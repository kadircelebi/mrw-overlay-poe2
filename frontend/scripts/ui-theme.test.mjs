import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import {applyThemePalette,paletteKeys} from '../public/ui-theme.mjs';
const presets=JSON.parse(fs.readFileSync(new URL('../../internal/uitheme/presets.json',import.meta.url),'utf8'));
function root(){const values=new Map();return {dataset:{},style:{setProperty:(k,v)=>values.set(k,v),removeProperty:k=>values.delete(k),getPropertyValue:k=>values.get(k)||'',values}}}
function apply(theme,extra={}){globalThis.document={documentElement:root()};const reference=presets.find(p=>p.id===theme.base).colors;const result=applyThemePalette({base:theme.base,colors:theme.colors,reference,grain:theme.grain,...extra});return {result,root:document.documentElement}}
test('every built-in has the exact editable role schema; game colors cannot be edited',()=>{
 for(const p of presets){assert.deepEqual(Object.keys(p.colors).sort(),[...paletteKeys].sort());assert.ok(p.readOnly)}
 for(const k of ['rare','unique','normal','magic','crafted','fractured','desecrated'])assert.ok(!paletteKeys.includes(k));
});
test('light cards get a cream background and outline; default restores original per-window shades',()=>{
 const {root:r}=apply(presets[2]);assert.equal(r.style.values.get('--ui-item-bg'),'#fffbf2');assert.equal(r.dataset.uiItemLight,'true');assert.equal(r.style.colorScheme,'light');
 applyThemePalette({base:'default',colors:presets[0].colors,reference:presets[0].colors,grain:true});assert.equal(r.style.values.has('--ui-bg'),false);assert.equal(r.style.values.has('--ui-item-bg'),false);assert.equal(r.style.values.has('--grain'),false);
});
test('a copy of default keeps unedited shades and updates only edited roles',()=>{
 const {root:r}=apply({...presets[0],colors:{...presets[0].colors,gold:'#123456'}});
 assert.equal(r.style.values.get('--gold'),'#123456');assert.equal(r.style.values.get('--ui-gold'),'#123456');assert.equal(r.style.values.has('--ui-bg'),false);
});
test('invalid palette cannot inject CSS; foreign roles are never applied',()=>{
 let r=apply(presets[2],{colors:{...presets[2].colors,bg:'red; background:url(x)'}});assert.equal(r.result,false);assert.equal(r.root.style.values.size,0);
 r=apply(presets[2],{colors:{...presets[2].colors,crafted:'#ff0000'}});assert.equal(r.result,true);assert.equal(r.root.style.values.has('--crafted'),false);assert.equal(r.root.style.values.has('--ui-crafted'),false);
});
test('an edited dark copy with a light item background gets the same readability treatment',()=>{
 const {root:r}=apply({...presets[1],colors:{...presets[1].colors,'item-bg':'#ffffff'}});assert.equal(r.dataset.uiItemLight,'true');assert.equal(r.style.colorScheme,'dark');
});
