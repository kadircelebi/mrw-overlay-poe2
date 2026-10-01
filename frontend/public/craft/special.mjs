import { candidates, chooseWeighted, roll, overlaps, limits, count, removeMod } from './engine.mjs';
import { filterOmenRows } from './omens.mjs';
const noOmens = {side:null,tags:[],quantity:1};

export function applicable(rule, data) {
  return !rule.beforeClassIds || rule.beforeClassIds.includes(data.options.ItemClassesCode);
}

export function essenceRows(data, rule) {
  const tags = new Set([data.options.ItemClassesCode.toLowerCase(), 'armour', data.options.tags]);
  return data.mods.filter(m => m.pool === rule.pool && m.name === rule.name &&
    (!m.spawn_tags.some(t => t !== 'default') || m.spawn_tags.some(t => tags.has(t))));
}

function availableEssences(item, data, rule) {
  return essenceRows(data, rule).filter(m => m.required_ilvl <= item.ilvl &&
    count(item, m.affix) < limits.Rare && !item.mods.some(other => overlaps(other, m)));
}

export function specialReason(item, data, rule, effects=noOmens) {
  if (item.reveal) return 'Önce bekleyen Desecrate seçimini tamamla veya geri al.';
  if (!applicable(rule, data)) return 'Bu currency eşya türüne uygun değil.';
  if (!rule.beforeRarity.includes(item.rarity)) return `${rule.beforeRarity.join(' / ')} eşya gerekiyor.`;
  if (rule.operation === 'desecrate') {
    if (item.mods.some(m => m.desecrated || m.pool === 'desecrated')) return 'Eşyada zaten Desecrated affix var.';
    if (rule.maxItemLevel && item.ilvl > rule.maxItemLevel) return `En fazla Item Level ${rule.maxItemLevel} gerekiyor.`;
    const possible = item.mods.length === 6 ? item.mods.some((m,i) => (!effects.side || m.affix === effects.side) && boneRows(removeMod(item,i),data,rule,effects).length) : boneRows(item,data,rule,effects).length;
    return possible ? '' : 'Uygun Desecrate mod havuzu yok.';
  }
  if (rule.removes) {
    if (!item.mods.length) return 'Çıkarılacak affix yok.';
    // Do not preselect a favourable removal: all random removals must be valid.
    return item.mods.every((_,i) => availableEssences(removeMod(item,i),data,rule).length) ? '' :
      'Rastgele çıkarma sonrası garantili mod için yer veya aile uygun olmayabilir; önce çakışan affix’i kaldır.';
  }
  return availableEssences(item,data,rule).length ? '' : 'Garantili mod için uygun seviye, mod ailesi veya boş yer yok.';
}

function boneRows(item,data,rule,effects=noOmens) {
  return filterOmenRows((effects.tags.length ? ['desecrated'] : ['normal','desecrated'])
    .flatMap(pool => candidates(item,data,{pool,minimum:rule.minimum || 1})),effects);
}

export function applySpecial(item, data, rule, random = Math.random, effects=noOmens) {
  const reason = specialReason(item,data,rule,effects); if (reason) throw new Error(reason);
  let next = structuredClone(item);
  if (rule.operation === 'essence') {
    if (rule.removes) next = removeMod(next,Math.floor(random()*next.mods.length));
    const rows = availableEssences(next,data,rule);
    // Multiple guaranteed attribute variants are equiprobable in this prototype.
    next.mods.push(roll(rows[Math.floor(random()*rows.length)],random));
    next.rarity = 'Rare'; return next;
  }
  if (next.mods.length === 6) {
    const removable = next.mods.map((m,i)=>({m,i})).filter(({m})=>!effects.side || m.affix === effects.side);
    next = removeMod(next,removable[Math.floor(random()*removable.length)].i);
  }
  let rows = boneRows(next,data,rule,effects);
  const first = chooseWeighted(rows,random), choices = [roll(first,random)];
  rows = rows.filter(m => m.affix === first.affix && !overlaps(m,first));
  while (choices.length < 3 && rows.length) {
    const chosen = chooseWeighted(rows,random); choices.push(roll(chosen,random));
    rows = rows.filter(m => !overlaps(m,chosen));
  }
  const index = next.mods.length;
  next.mods.push({source_id:'local-unrevealed',pool:'desecrated',affix:first.affix,name:'Unrevealed',tier:'?',
    families:['Unrevealed'],required_ilvl:1,text:'Unrevealed Desecrated modifier',ranges:[],values:[],desecrated:true});
  next.reveal = {index,choices,effects:structuredClone(effects)}; return next;
}

export function revealChoice(item,index) {
  if (!item.reveal || !item.reveal.choices[index]) throw new Error('Geçersiz Desecrate seçimi.');
  const next = structuredClone(item);
  next.mods[next.reveal.index] = {...next.reveal.choices[index],desecrated:true};
  delete next.reveal; return next;
}
