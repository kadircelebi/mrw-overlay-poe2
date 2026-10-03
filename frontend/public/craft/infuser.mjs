import { t } from './i18n.mjs';
// Quality limits and the Vaal Infusers.
//
// Maximum quality is 20%, more on some rings: Breach Ring's implicit adds
// 20% and Refined Breach Ring's 25% (RePoE base_items), and Essence of the
// Breach's "+20% to Maximum Quality" stacks on top (checked by the player:
// Breach Ring 40, with the essence 60, Infusers up to 75 on a Refined one).
//
// A Vaal Infuser works only at or above the maximum and goes up to 10 points
// past it. Each use either adds quality (1%, at item level 82+ 2% about one
// time in six) or corrupts the item, which then only stops changing (Maxroll:
// it does not reroll anything like a Vaal Orb). The odds are community
// measurements, not GGG's: no corruption at the maximum, then about 5% more
// per point above it (~45% at 9 points above).

const art = name => `Art/2DItems/Currency/IncursionCraftingOrbs/${name}.webp`;
export const infusers = {
  'vaal-armourers-infuser': { name: "Vaal Armourer's Infuser", short: 'Armourer', icon: art('VaakArmourersScrap'),
    classes: ['Body Armour', 'Boots', 'Gloves', 'Helmet', 'Shield', 'Buckler', 'Focus'] },
  'vaal-blacksmiths-infuser': { name: "Vaal Blacksmith's Infuser", short: 'Blacksmith', icon: art('VaalBlacksmithsWhetstone'),
    classes: ['Bow', 'Crossbow', 'One Hand Mace', 'Two Hand Mace', 'Spear', 'Warstaff', 'Talisman'] },
  'vaal-arcanists-infuser': { name: "Vaal Arcanist's Infuser", short: 'Arcanist', icon: art('VaalArcanistsEtcher'),
    classes: ['Wand', 'Staff', 'Sceptre'] },
  'vaal-catalysing-infuser': { name: 'Vaal Catalysing Infuser', short: 'Catalysing', icon: art('VaalCatalyst'),
    classes: ['Ring', 'Amulet'] },
};
export const isInfuser = id => Object.hasOwn(infusers, id);
export const infuserBeyond = 10;

const baseBonus = { 'Breach Ring': 20, 'Refined Breach Ring': 25 };
const maxQualityRE = /\+(\d+)% to Maximum Quality/i;

// The item's maximum quality before any Infuser.
export function maxQuality(item) {
  let max = 20 + (baseBonus[item.baseName] || 0);
  for (const mod of item.mods || []) {
    const m = maxQualityRE.exec(mod.text || '');
    if (m) max += Number(m[1]);
  }
  return max;
}

export const infuserRules = () => Object.fromEntries(Object.entries(infusers).map(([id, x]) => [id, {
  name: x.name, short: x.short, icon: x.icon, tier: 'Vaal', type: 'Currency',
  beforeRarity: ['Normal', 'Magic', 'Rare'], afterTrigger: 'infuse', operation: 'infuse', classes: x.classes,
}]));

// Jewellery quality is a catalyst's; other items' is plain quality.
const jewellery = data => ['Ring', 'Amulet'].includes(data.options?.ItemClassesCode);
export const currentQuality = (item, data) => jewellery(data) ? item.catalyst?.quality || 0
  : Number.isInteger(item.quality) ? item.quality : 20;

export const corruptChance = (quality, max) => Math.min(1, Math.max(0, quality - max) * 0.05);

export function infuseReason(item, data, id) {
  const x = infusers[id];
  if (!x.classes.includes(data.options?.ItemClassesCode)) return t('err.infuserClass', x.classes.join(', '));
  const q = currentQuality(item, data), max = maxQuality(item);
  if (jewellery(data) && !item.catalyst?.id) return t('err.infuserNeedsCatalyst');
  if (q < max) return t('err.infuserBelowMax', max);
  if (q >= max + infuserBeyond) return t('err.infuserCap', max + infuserBeyond);
  return '';
}

export function applyInfuser(item, data, id, random = Math.random) {
  const q = currentQuality(item, data), max = maxQuality(item);
  if (random() < corruptChance(q, max)) return { ...item, corrupted: true };
  const step = item.ilvl >= 82 && random() < 1 / 6 ? 2 : 1;
  const next = Math.min(max + infuserBeyond, q + step);
  return jewellery(data) ? { ...item, catalyst: { ...item.catalyst, quality: next } } : { ...item, quality: next };
}
