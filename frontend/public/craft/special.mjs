import { t } from './i18n.mjs';
import { candidates, chooseWeighted, roll, overlaps, count, removeMod, removable, sideLimit, isFull, isCrafted, craftedLimit } from './engine.mjs';
import { filterOmenRows } from './omens.mjs';
const noOmens = {side:null,tags:[],quantity:1};

// The essences the craft offers. Greater: a Magic item becomes Rare with the
// guaranteed modifier, its affixes stay. Perfect (and the corrupted ones like
// Hysteria, which work the same way): on a Rare, a random affix is removed and
// the guaranteed modifier added. Lesser and plain essences are left out.
export function essenceTier(id, rule) {
  if (rule.operation !== 'essence') return null;
  if (id.startsWith('greater-')) return 'greater';
  if (id.startsWith('perfect-')) return 'perfect';
  return rule.pool === 'perfect_essence' ? 'special' : null;
}

export function applicable(rule, data) {
  return !rule.beforeClassIds || rule.beforeClassIds.includes(data.options.ItemClassesCode);
}

// data.tags are the tags every base of the class carries (from the data
// build), so an essence row fits jewellery and weapons as well as armour.
export function essenceRows(data, rule) {
  const tags = new Set(data.tags || [data.options.ItemClassesCode.toLowerCase(), 'armour', data.options.tags]);
  return data.mods.filter(m => m.pool === rule.pool && m.name === rule.name &&
    (!m.spawn_tags.some(t => t !== 'default') || m.spawn_tags.some(t => tags.has(t))));
}

// An item takes one crafted (essence) modifier, two with Astrid's Creativity.
function availableEssences(item, data, rule) {
  if (item.mods.filter(isCrafted).length >= craftedLimit(item)) return [];
  return essenceRows(data, rule).filter(m => m.required_ilvl <= item.ilvl &&
    count(item, m.affix) < sideLimit(item, m.affix, 'Rare') && !item.mods.some(other => overlaps(other, m)));
}

export function specialReason(item, data, rule, effects=noOmens) {
  if (item.reveal) return t('err.pendingReveal');
  if (item.sanctified) return t('err.sanctified');
  if (item.corrupted) return t('err.corrupted');
  if (!applicable(rule, data)) return t('err.notForClass');
  if (!rule.beforeRarity.includes(item.rarity)) return t('err.needRarity', rule.beforeRarity.join(' / '));
  if (rule.operation === 'desecrate') {
    if (item.mods.some(m => m.desecrated || m.pool === 'desecrated')) return t('err.hasDesecrated');
    if (rule.maxItemLevel && item.ilvl > rule.maxItemLevel) return t('err.maxIlvl', rule.maxItemLevel);
    const possible = isFull(item) ? removable(item,{side:effects.side}).some(i => boneRows(removeMod(item,i),data,rule,effects).length) : boneRows(item,data,rule,effects).length;
    return possible ? '' : t('err.noDesecratePool');
  }
  if (item.mods.filter(isCrafted).length >= craftedLimit(item)) return t('err.craftedLimit', craftedLimit(item));
  if (rule.removes) {
    // A fractured affix is never the one removed; Crystallisation omens pick the side.
    const options = removable(item,effects.removal || {});
    if (!options.length) return t('err.nothingToRemove');
    // Do not preselect a favourable removal: all random removals must be valid.
    return options.every(i => availableEssences(removeMod(item,i),data,rule).length) ? '' :
      t('err.essenceRemoveRisk');
  }
  return availableEssences(item,data,rule).length ? '' : t('err.essenceNoRoom');
}

export function boneRows(item,data,rule,effects=noOmens) {
  return filterOmenRows((effects.tags.length ? ['desecrated'] : ['normal','desecrated'])
    .flatMap(pool => candidates(item,data,{pool,minimum:rule.minimum || 1})),effects);
}

export function applySpecial(item, data, rule, random = Math.random, effects=noOmens) {
  const reason = specialReason(item,data,rule,effects); if (reason) throw new Error(reason);
  let next = structuredClone(item);
  if (rule.operation === 'essence') {
    if (rule.removes) { const options = removable(next,effects.removal || {}); next = removeMod(next,options[Math.floor(random()*options.length)]); }
    const rows = availableEssences(next,data,rule);
    // Multiple guaranteed attribute variants are equiprobable in this prototype.
    next.mods.push(roll(rows[Math.floor(random()*rows.length)],random));
    next.rarity = 'Rare'; return next;
  }
  if (isFull(next)) {
    const options = removable(next,{side:effects.side});
    next = removeMod(next,options[Math.floor(random()*options.length)]);
  }
  // The bone decides the side; the options come only when the modifier is
  // revealed at the Well of Souls. Until then the item is used as it is (a
  // common trick: Fracturing then cannot pick the unrevealed modifier).
  const first = chooseWeighted(boneRows(next,data,rule,effects),random);
  next.mods.push({source_id:'local-unrevealed',pool:'desecrated',affix:first.affix,name:'Unrevealed',tier:'?',
    families:['Unrevealed'],required_ilvl:1,text:'Unrevealed Desecrated modifier',ranges:[],values:[],desecrated:true,
    unrevealed:true,revealWith:{effects:structuredClone(effects),minimum:rule.minimum || 1}});
  return next;
}

export const unrevealedIndex = item => item.mods.findIndex(m => m.unrevealed);

// Up to three options from distinct families on the unrevealed modifier's
// side, weighted again on every roll.
function revealOptions(item,data,index,random) {
  const mod = item.mods[index], ctx = mod.revealWith || {effects:noOmens,minimum:1};
  let rows = boneRows(removeMod(item,index),data,{minimum:ctx.minimum},ctx.effects).filter(m => m.affix === mod.affix);
  const choices = [];
  while (choices.length < 3 && rows.length) {
    const chosen = chooseWeighted(rows,random); choices.push(roll(chosen,random));
    rows = rows.filter(m => !overlaps(m,chosen));
  }
  if (!choices.length) throw new Error(t('err.noDesecratePool'));
  return choices;
}

// echo: Omen of Abyssal Echoes, used up on the reveal; it allows one reroll.
export function startReveal(item,data,{echo=false}={},random=Math.random) {
  if (item.reveal) throw new Error(t('err.pendingReveal'));
  const index = unrevealedIndex(item);
  if (index < 0) throw new Error(t('err.nothingToReveal'));
  return {...structuredClone(item),reveal:{index,choices:revealOptions(item,data,index,random),echo,rerolled:false}};
}

export function rerollReveal(item,data,random=Math.random) {
  if (!item.reveal?.echo || item.reveal.rerolled) throw new Error(t('err.noEcho'));
  const next = structuredClone(item);
  next.reveal.choices = revealOptions(item,data,item.reveal.index,random);
  next.reveal.rerolled = true;
  return next;
}

export function revealChoice(item,index) {
  if (!item.reveal || !item.reveal.choices[index]) throw new Error(t('err.badReveal'));
  const next = structuredClone(item);
  next.mods[next.reveal.index] = {...next.reveal.choices[index],desecrated:true};
  delete next.reveal; return next;
}
