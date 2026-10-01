import { rolledText, sortedMods } from './engine.mjs';

// Generic gloves deliberately search the category, not a fabricated base name.
export function craftText(item) {
  if (item.reveal) throw new Error('Önce bekleyen Desecrate seçimini tamamla.');
  if (!['Normal','Magic','Rare'].includes(item.rarity)) throw new Error('Geçersiz rarity.');
  const lines = ['Item Class: Gloves',`Rarity: ${item.rarity}`];
  if (item.rarity === 'Rare') lines.push('Theoretical Craft');
  lines.push('Gloves','--------',`Item Level: ${item.ilvl}`,'--------');
  for (const {mod} of sortedMods(item)) {
    lines.push(`{ ${mod.desecrated || mod.pool === 'desecrated' ? 'Desecrated ' : ''}${mod.affix} Modifier (Tier: ${mod.tier}) }`);
    lines.push(...rolledText(mod).split(/\n|<br\s*\/?\s*>/i));
  }
  return lines.join('\n');
}
