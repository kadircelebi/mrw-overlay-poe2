import { createItem, count, sideLimit, isCrafted, isDesecrated, craftedLimit, manualAdd, manualReason, removeMod, clearMods,
  setRarity, candidates, currencyReason, applyCurrency, supported, rolledText, replaceTier, sortedMods, rollPools, fracturable, removable } from './engine.mjs';
import { applicable, essenceRows, essenceTier, specialReason, applySpecial, revealChoice, startReveal, rerollReveal, unrevealedIndex } from './special.mjs';
import { usageEntry, summarize } from './ledger.mjs';
import { craftText } from './trade.mjs';
import { iconIndex, iconFor } from './icons.mjs';
import { applyThemePalette } from '../ui-theme.mjs';
import { catalystRules, catalystBoost, augmentedValues, catalystFromCopy, catalystTypes, catalystClasses } from './catalyst.mjs';
import { infuserRules, maxQuality, infuserBeyond, currentQuality, corruptChance } from './infuser.mjs';
import { omenDefinitions, relevantOmens, omenEffects, filterOmenRows, orbOmenReason, applyOrbOmens } from './omens.mjs';
import { t, setLang, locale, num, variantName } from './i18n.mjs';
import { pageFor, importItem, runesFor, runeStatLines } from './import.mjs';
import { baseStats, requirementLine } from './stats.mjs';
import { lineKey, targetOf, hasTarget, targetReason, fastRunner, stats, histogram } from './simulate.mjs';
import { corrupt, corruptReason, corruptOutcomes } from './corrupt.mjs';
import { parseLibrary, entryFor, sameCraft, addEntry, removeEntry } from './library.mjs';
import { orbOdds, desecrateOdds, revealOdds } from './odds.mjs';

const $ = id => document.getElementById(id);
const clone = object => structuredClone(object);
const shortNames = { transmute: 'Transmutation', aug: 'Augmentation', regal: 'Regal', exalted: 'Exalted', chaos: 'Chaos', annu: 'Annulment', divine: 'Divine', fracture: 'Fracturing', vaal: 'Vaal' };
const order = ['transmute', 'aug', 'regal', 'exalted', 'chaos', 'annu', 'divine', 'fracture', 'vaal'];
const basePools = ['normal', 'desecrated', 'essence', 'perfect_essence'];
let item = createItem(), datasets = {}, rules = {}, selected = 'transmute', tier = '', pool = 'normal';
let history = [], undo = [], ready = false;
let runes = {}, specials = {}, specialSelected = 'preserved-rib', prices = {currency:[]}, priceOverrides = {}, archives = [], sessionStart = new Date().toISOString();
// Legacy archives remain in saved/exported data, but are no longer created or displayed.
const storageKey = 'mrw-craft-v1';
let mode = 'basic', omens = {}, activeOmens = {basic:[],desecrate:[],essence:[]};
// In the game an omen is used up with the currency; keepOmens leaves it
// ticked for the next press (each press still pays for one).
let keepOmens = false;
const specialRemember = {desecrate:'preserved-rib',essence:'greater-essence-of-the-mind'};
// Item classes and their defence variants; item.base is the data page
// ("Gloves_str", "Rings"), so drafts saved before other classes still load.
let classes = [], pages = {}, bases = {}, manifest = null;
// The game bases of each page (bases.json): an item may name one in
// baseName; without it (drafts from before, or a class switch) the highest
// level base of the page stands in. Quality defaults to 20, like a base
// worked on before crafting.
let baseData = { pages: {} };
const baseList = page => baseData.pages[page] || [];
const currentBase = (state = item) => baseList(state.base).find(b => b.name === state.baseName) || baseList(state.base).at(-1) || null;
// Rings and amulets have no plain quality, only a catalyst's (item.catalyst).
const jewelleryPages = ['Rings', 'Amulets'];
const qualityOf = state => jewelleryPages.includes(state.base) ? 0 : Number.isInteger(state.quality) ? state.quality : 20;
const snapshot = state => ({base:state.base,baseName:currentBase(state)?.name || '',quality:qualityOf(state),catalyst:state.catalyst ? `${state.catalyst.id}:${state.catalyst.quality}` : '',rarity:state.rarity,ilvl:state.ilvl,runes:runesOf(state).join(','),sockets:socketsOf(state),runeStats:(state.runeStats || []).join('|'),
  mods:state.mods.map(m => ({source_id:m.source_id,pool:m.pool,affix:m.affix,tier:m.tier,values:m.values,desecrated:Boolean(m.desecrated),fractured:Boolean(m.fractured),crafted:Boolean(m.crafted)}))});
// Augment sockets: Craft of Exile's normal maximum (body armour and two-hand
// weapons 2, other equipment 1) plus one more, which corruption and some
// items reach (a Hate Palm gloves has two). Jewellery and quivers have none.
const twoHanded = new Set(['body', 'bow', 'crossbow', 'twomace', 'warstaff', 'staff', 'talisman']);
const maxSockets = page => {
  const id = pages[page]?.cls.id;
  return !id || ['amulet', 'ring', 'belt', 'quiver'].includes(id) ? 0 : twoHanded.has(id) ? 3 : 2;
};
// A Vaal Orb adds a socket past that limit (an exceptional bow with three
// becomes four), so a Corrupted item may hold one more.
const socketCap = state => maxSockets(state.base) ? maxSockets(state.base) + (state.corrupted ? 1 : 0) : 0;
const socketsOf = state => Math.min(socketCap(state),
  Number.isInteger(state.sockets) ? state.sockets : (state.runes || []).filter(Boolean).length || (state.rune ? 1 : 0));
// The rune in each socket ('' = empty or a rune that does not matter to
// crafting). Drafts from before sockets had a single item.rune.
const runesOf = state => {
  const list = Array.isArray(state.runes) ? [...state.runes] : state.rune ? [state.rune] : [];
  const n = socketsOf(state);
  while (list.length < n) list.push('');
  return list.slice(0, n);
};
// What the socketed runes do, kept on the item for the engine: extra pools
// to roll from, one more suffix (Serle's), one more crafted modifier (Astrid's).
function withRunes(state, list) {
  const placed = list.map(id => runes[id]).filter(Boolean);
  const next = { ...state, runes: list, runePools: placed.filter(r => r.kind === 'pool').map(r => r.pool),
    suffixBonus: placed.some(r => r.kind === 'suffix') ? 1 : 0, craftedLimit: placed.some(r => r.kind === 'crafted') ? 2 : 1 };
  delete next.rune; delete next.runePool;
  return next;
}
const runeShort = r => r.kind === 'pool' ? r.label : r.kind === 'suffix' ? '+1 Suffix' : '+1 Crafted';
// The runes of the current page ("Gloves: Can roll Marksman modifiers").
const pageRunes = (page = item.base) => Object.entries(runes).filter(([, r]) => r.pages.includes(page));
function persist() {
  try { localStorage.setItem(storageKey,JSON.stringify({format:2,item:simPending ? simPending.before : item,history,archives,priceOverrides,sessionStart,mode,activeOmens,selected,tier,specialSelected,specialRemember,pool,keepOmens,
    sim:{target:simTarget,orb:simOrb,omens:simOmens}})); }
  catch { status(t('storage.failed'),'error'); }
}

function element(tag, text, className) {
  const node = document.createElement(tag);
  if (text !== undefined) node.textContent = text;
  if (className) node.className = className;
  return node;
}
function status(text, kind = '') { $('status').textContent = text; $('status').className = kind; }
function commit(next, label, usage = null, newSession = false) {
  // A running or unkept simulation owns the item card.
  if (simBusy || simPending) return;
  if (newSession && !history.length && JSON.stringify(snapshot(item)) === JSON.stringify(snapshot(next))) return;
  undo.push({ item: clone(item), history: clone(history), sessionStart,activeOmens:clone(activeOmens) });
  if (undo.length > 100) undo.shift();
  const before = snapshot(item);
  if (newSession) {
    autoSaveCurrent();
    history = []; sessionStart = new Date().toISOString();
  }
  // The base's affix limits travel with the item for the engine.
  item = { ...next, baseSlots: currentBase(next)?.slots || null };
  const usages = Array.isArray(usage) ? usage : usage ? [usage] : [];
  if (!newSession) history.unshift({ time: new Date().toISOString(), label, usages, before, after:snapshot(next) });
  if (!keepOmens) for (const context of Object.keys(activeOmens)) activeOmens[context] = activeOmens[context].filter(id => !usages.some(u => u.id === id));
  render();
  status(label, 'success');
  persist();
}
function attempt(action) { try { action(); } catch (error) { status(error.message, 'error'); } }
const data = () => datasets[item.base];
const classOf = page => pages[page]?.cls;
const itemLabel = page => {
  const info = pages[page];
  return info ? info.cls.itemClass + (info.attr ? ` · ${variantName(info.attr)}` : '') : page;
};
// Every tier is on screen at once (Standard, Greater, Perfect rows), so a
// craft like Annul → Perfect Exalt → Annul needs no tab switching.
const tiers = ['', 'II', 'III'];
const visibleRules = () => Object.entries(rules).filter(([id]) => supported(id))
  .sort(([a, x], [b, y]) => tiers.indexOf(x.tier || '') - tiers.indexOf(y.tier || '') || order.indexOf(kind(a)) - order.indexOf(kind(b)));
function kind(id) { return id.includes('transmutation') ? 'transmute' : id.includes('augmentation') ? 'aug' : id.includes('fracturing') ? 'fracture' : order.find(k => id.includes(k)) || id; }

async function readJSON(path) {
  const response = await fetch(path);
  if (!response.ok) throw new Error(t('load.fileFailed', path, response.status));
  return response.json();
}
// A class's modifiers load the first time it is picked.
async function load(page) {
  if (!datasets[page]) datasets[page] = await readJSON(`data/${page}.mods.json`);
  return datasets[page];
}

// Texts in the page itself carry data-i18n keys.
function applyStatic() {
  document.querySelectorAll('[data-i18n]').forEach(n => { n.textContent = t(n.dataset.i18n); });
  document.querySelectorAll('[data-i18n-placeholder]').forEach(n => { n.placeholder = t(n.dataset.i18nPlaceholder); });
  document.querySelectorAll('[data-i18n-aria-label]').forEach(n => n.setAttribute('aria-label', t(n.dataset.i18nAriaLabel)));
  document.title = `MrW · ${t('title')}`;
  if (manifest) $('source-info').textContent = t('source.info', new Date(manifest.fetched_at_utc).toLocaleDateString(locale()));
}

function renderClassPicker() {
  const info = pages[item.base];
  const classSelect = $('item-class'), variantSelect = $('variant');
  if (classSelect.options.length !== classes.length) {
    classSelect.replaceChildren(...classes.map(c => new Option(c.itemClass, c.id)));
  }
  classSelect.value = info?.cls.id || '';
  const variants = info?.cls.variants || [];
  variantSelect.replaceChildren(...variants.map(v => new Option(variantName(v.attr), v.page)));
  variantSelect.value = item.base;
  $('variant-field').hidden = variants.length < 2;
  const base = currentBase();
  $('base-label').textContent = base ? `${base.name} · ${base.level}` : '—';
  $('base').disabled = !baseList(item.base).length || Boolean(item.reveal);
  if ($('base').disabled) closeBasePicker();
  // Sockets first; a modifier rune needs one, and only one such rune fits an
  // item. It is socket-bound: once in, it stays until a new craft.
  const most = socketCap(item), sockets = socketsOf(item);
  $('socket-field').hidden = !most;
  $('sockets').replaceChildren(...Array.from({ length: most + 1 }, (_, n) => new Option(String(n), String(n))));
  $('sockets').value = String(sockets);
  $('sockets').disabled = Boolean(item.reveal);
  // One select per socket. Each rune fits once ("Limited to 1"); a
  // socket-bound one stays, Astrid's can only be swapped for another rune.
  const available = pageRunes(), list = runesOf(item);
  $('socket-slots').replaceChildren(...list.map((id, i) => {
    const wrap = element('div'), select = element('select'), label = element('label', t('socket.n', i + 1));
    select.id = `socket-${i}`; label.htmlFor = select.id;
    // Astrid's Creativity can be swapped for any rune, an ordinary one too
    // (shown as no craft rune): the usual way to keep a second crafted
    // modifier after crafting it. A socket-bound rune cannot be swapped.
    const empty = new Option(id ? t('rune.ordinary') : t('rune.none'), ''); empty.disabled = Boolean(id && runes[id]?.bound);
    select.append(empty, ...available.map(([rid, r]) => {
      const option = new Option(`${r.name} · ${runeShort(r)}`, rid);
      option.disabled = list.includes(rid) && rid !== id;
      return option;
    }));
    select.value = id;
    select.disabled = Boolean(item.reveal) || Boolean(id && runes[id]?.bound);
    select.title = id && runes[id] ? runes[id].text : '';
    select.onchange = () => setRune(i, select.value);
    wrap.append(label, select);
    return wrap;
  }));
  $('rune-hint').textContent = !sockets ? (available.length ? t('rune.needSocket') : '')
    : list.some(id => runes[id]?.bound) ? t('rune.bound') : t('rune.hint');
  $('source-link').href = `https://poe2db.tw/us/${encodeURIComponent(item.base)}#ModifiersCalc`;
}

function renderItem() {
  renderClassPicker();
  $('rarity').value = item.rarity;
  $('ilvl').value = item.ilvl;
  $('quality').value = qualityOf(item);
  for (const id of ['rarity','item-class','variant','ilvl','quality','clear','reset']) $(id).disabled = Boolean(item.reveal);
  // Jewellery quality comes only from catalysts.
  if (jewelleryPages.includes(item.base)) { $('quality').disabled = true; $('quality').title = t('quality.catalystOnly'); } else $('quality').title = '';
  $('item-card').className = 'item-card ' + item.rarity;
  const base = currentBase();
  $('item-card').classList.toggle('corrupted', Boolean(item.corrupted));
  $('item-card').classList.toggle('sanctified', Boolean(item.sanctified));
  $('item-state').textContent = item.sanctified ? 'Sanctified' : item.corrupted ? 'Corrupted' : '';
  $('item-state').hidden = !item.corrupted && !item.sanctified;
  $('item-name').textContent = base?.name || itemLabel(item.base);
  $('item-class-label').textContent = itemLabel(item.base);
  $('rarity-label').textContent = item.rarity;
  renderProps(base);
  $('prefix-count').textContent = `${count(item, 'Prefix')} / ${sideLimit(item, 'Prefix')}`;
  $('suffix-count').textContent = `${count(item, 'Suffix')} / ${sideLimit(item, 'Suffix')}`;
  $('crafted-count').textContent = `${item.mods.filter(isCrafted).length} / ${craftedLimit(item)}`;
  // More crafted modifiers than the limit is legal (crafted with Astrid's
  // Creativity, which was then swapped out); no more can be added.
  $('crafted-summary').classList.toggle('full', item.mods.filter(isCrafted).length >= craftedLimit(item));
  $('crafted-summary').title = t('crafted.hint');
  const mods = $('item-mods'); mods.replaceChildren();
  // A corruption enchantment sits above the affixes, as in the game.
  if (item.enchant) {
    const row = element('div', undefined, 'item-enchant');
    row.append(element('p', rolledWithRange(item.enchant), 'mod-value'));
    row.title = `${item.enchant.name} · Enchant`;
    mods.append(row);
  }
  if (!item.mods.length) mods.append(element('p', item.rarity === 'Rare' ? t('empty.rare') : t('empty.other'), 'empty-item'));
  // Laid out like the game's tooltip: prefixes then suffixes, the side named
  // once on the left, the tier on the right (C for crafted), the roll with its
  // range. A click on a line opens its roll editor and remove button.
  let lastSide = '';
  sortedMods(item).forEach(({ mod, index }) => {
    const kind = mod.unrevealed ? 'unrevealed' : mod.fractured ? 'fractured' : isCrafted(mod) ? 'crafted' : isDesecrated(mod) ? 'desecrated' : 'explicit';
    const row = element('div', undefined, `item-mod ${kind}${isDesecrated(mod) && kind !== 'desecrated' ? ' desecrated-bg' : ''}`);
    row.dataset.index = String(index);
    const key = `${mod.affix}:${mod.source_id}`;
    row.append(element('span', mod.affix !== lastSide ? mod.affix : '', 'mod-side'));
    lastSide = mod.affix;
    // Catalyst quality raises matching values, shown as the game does.
    const raised = mod.unrevealed ? null : augmentedValues(mod, item);
    // Like the game's tooltip, the raised value stands alone; the roll and
    // its range go to the hover text.
    const text = element('p', mod.unrevealed ? t('mod.unrevealed') : raised ? rolledText({ ...mod, values: raised }) : rolledWithRange(mod), `mod-value${raised ? ' augmented' : ''}`);
    text.title = `${mod.name} · ${mod.desecrated ? 'Desecrated' : mod.pool}${mod.fractured ? ' · Fractured' : ''}${raised ? ` · ${rolledWithRange(mod)} + ${item.catalyst.quality}%` : ''}`;
    row.append(text, element('span', isCrafted(mod) ? 'C' : `T${mod.tier}`, 'mod-tier'));
    row.onclick = event => {
      if (held || event.target.closest('.mod-editor')) return;
      openMod = openMod === key ? '' : key; renderItem();
    };
    if (openMod === key && !mod.unrevealed) {
      const editor = element('div', undefined, 'mod-editor');
      editor.append(element('small', text.title));
      mod.ranges.forEach((range, rangeIndex) => {
        const input = element('input'); input.type = 'number'; input.min = range.min; input.max = range.max;
        input.step = Number.isInteger(range.min) && Number.isInteger(range.max) ? '1' : '.01';
        input.value = mod.values[rangeIndex];
        input.disabled = Boolean(item.reveal);
        const id = `roll-${index}-${rangeIndex}`; input.id = id;
        const label = element('label', `${range.min}–${range.max}`); label.htmlFor = id;
        input.setAttribute('aria-label', `${mod.name} roll ${rangeIndex + 1} (${range.min}–${range.max})`);
        input.onchange = () => attempt(() => {
          const value = Number(input.value);
          if (input.value === '' || !Number.isFinite(value) || value < range.min || value > range.max ||
              (input.step === '1' && !Number.isInteger(value))) {
            input.setAttribute('aria-invalid', 'true'); throw new Error(t('roll.range', range.min, range.max));
          }
          const next = clone(item); next.mods[index].values[rangeIndex] = value;
          commit(next, t('roll.set', mod.name, value));
        });
        editor.append(input, label);
      });
      const remove = element('button', t('mod.remove'), 'remove');
      remove.setAttribute('aria-label', t('mod.removeAria', mod.name));
      remove.disabled = Boolean(item.reveal);
      remove.onclick = () => { openMod = ''; commit(removeMod(item, index), t('mod.removed', mod.name, item.rarity)); };
      editor.append(remove);
      row.append(editor);
    }
    mods.append(row);
  });
  $('clear').disabled = !item.mods.length || Boolean(item.reveal);
  $('undo').disabled = !undo.length;
  $('reset').disabled = Boolean(item.reveal) || (item.rarity === 'Normal' && !item.mods.length && !history.length);
  const visibleHistory = history;
  const cost = summarize(visibleHistory);
  // Big totals read in Divine: the current Divine price converts them (the
  // operations keep the Exalted prices of when they were used).
  const divine = divineEx();
  $('cost-total').textContent = `${exText(cost.total)} Exalted${divine ? ` · ${num(cost.total / divine, 2)} Divine` : ''}${cost.unknown ? t('cost.unknown', cost.unknown) : ''}`;
  $('cost-rate').textContent = divine ? t('cost.rate', exText(divine)) : '';
  $('price-info').textContent = prices.generated_at ? t('price.info', prices.league, new Date(prices.generated_at).toLocaleString(locale())) : t('price.waiting');
  $('price-check').disabled = Boolean(item.reveal);
  $('cost-breakdown').replaceChildren();
  for (const row of cost.rows) $('cost-breakdown').append(element('li',`${row.quantity} × ${row.name} · ${exText(row.cost_ex)} Ex` +
    `${divine && row.cost_ex >= divine ? ` (${num(row.cost_ex / divine, 2)} div)` : ''}${row.unknown ? t('cost.missing') : ''}`));
  $('history').replaceChildren();
  for (const entry of visibleHistory) {
    const li = element('li'); const time = element('time', new Date(entry.time).toLocaleTimeString(locale(), { hour: '2-digit', minute: '2-digit' }));
    time.dateTime = entry.time; li.append(time, document.createTextNode(entry.label));
    for (const usage of entry.usages || (entry.usage ? [entry.usage] : [])) li.append(element('small',` · ${usage.quantity} × ${usage.name}: ${usage.unit_ex === null ? t('price.none') : exText(usage.unit_ex)+' Ex'}`));
    $('history').append(li);
  }
  if (!visibleHistory.length) $('history').append(element('li', t('history.empty')));
}

// The base's numbers as the game would show them; a number that its
// modifiers or quality changed is coloured like the game's augmented values.
function renderProps(base) {
  const props = $('item-props'); props.replaceChildren();
  const stats = baseStats(base, { ...item, quality: qualityOf(item) });
  const plain = baseStats(base, { mods: [], quality: 0 });
  const line = (label, value, changed, className) => {
    const row = element('div', undefined, className);
    row.append(document.createTextNode(`${label}: `), element('b', String(value), changed ? 'changed' : ''));
    props.append(row);
  };
  if (stats) {
    if (stats.quality) line('Quality', `+${stats.quality}%`, true);
    if (stats.block) line('Block chance', `${stats.block}%`, stats.block !== plain.block);
    if (stats.ar) line('Armour', stats.ar, stats.ar !== plain.ar);
    if (stats.ev) line('Evasion Rating', stats.ev, stats.ev !== plain.ev);
    if (stats.es) line('Energy Shield', stats.es, stats.es !== plain.es);
    if (stats.phys) {
      line('Physical Damage', `${stats.phys[0]}-${stats.phys[1]}`, stats.phys.join() !== plain.phys.join());
      for (const e of stats.elemental) line(`${e.kind} Damage`, `${e.min}-${e.max}`, true);
      line('Critical Hit Chance', `${stats.crit.toFixed(2)}%`, stats.crit !== plain.crit);
      line('Attacks per Second', stats.aps.toFixed(2), stats.aps !== plain.aps);
      line('DPS', `${num(stats.dps, 1)} (P ${num(stats.pdps, 1)} · E ${num(stats.edps, 1)})`, false, 'dps');
    }
  }
  if (item.catalyst?.quality) line(`Quality (${catalystTypes[item.catalyst.id]?.label || '?'} Modifiers)`, `+${item.catalyst.quality}%`, true);
  if (socketsOf(item)) line('Sockets', Array(socketsOf(item)).fill('S').join(' '), false);
  line('Item Level', item.ilvl, false);
  const runeLines = $('item-runes'); runeLines.replaceChildren();
  // A base that moves the affix limits shows it as its implicit does in game.
  (base?.slots || []).forEach((change, i) => {
    if (!change) return;
    const side = i ? 'Suffix' : 'Prefix';
    runeLines.append(element('div', `${change > 0 ? '+' : ''}${change} ${side} Modifier${Math.abs(change) > 1 ? 's' : ''} allowed`, 'rune'));
  });
  for (const text of item.runeStats || []) runeLines.append(element('div', text, 'rune'));
  for (const id of runesOf(item)) if (runes[id]) {
    const line = element('div', runes[id].text, 'rune'); line.title = runes[id].name; runeLines.append(line);
  }
  const req = base ? requirementLine(base.req || {}) : '';
  if (req) props.append(element('div', req, 'req'));
}

// Exalted amounts: whole numbers once they are large, more digits for cheap
// currency (a Transmutation is a fraction of an Exalted).
const exText = value => num(value, value >= 100 ? 0 : value >= 1 ? 2 : 4);
const divineEx = () => {
  const value = prices.currency?.find(r => r.api_id === 'divine' || r.name === 'Divine Orb')?.value_ex;
  return Number.isFinite(value) && value > 0 ? value : null;
};

// "99(92-100)%": the roll with its range, as the game's advanced copy shows it.
const rolledWithRange = mod => {
  let i = 0;
  return mod.text.replace(/\((-?\d+(?:\.\d+)?)[—–](-?\d+(?:\.\d+)?)\)/g, (all, min, max) => {
    const value = mod.values?.[i++];
    return value === undefined ? all : min === max ? String(value) : `${value}(${min}-${max})`;
  }).replace(/<br\s*\/?\s*>/gi, '\n');
};
let openMod = '';

function payment(id,rule) {
  const usage = usageEntry(id,rule.name,prices);
  if (Object.hasOwn(priceOverrides,id)) { usage.unit_ex = priceOverrides[id]; usage.source = 'user'; }
  return usage;
}
let icons = null;
// Icons load from the trade site (see icons.mjs); until they arrive, or with
// no connection, a currency button shows ◇ and other places no picture.
function setIcon(img, icon) {
  const src = iconFor(icons, icon);
  img.onerror = () => { img.hidden = true; };
  img.hidden = !src;
  if (src) img.src = src; else img.removeAttribute('src');
}
function iconElement(icon, orb = false) {
  const fallback = () => element('span', '◇', 'orb-fallback');
  const src = iconFor(icons, icon);
  if (!src && orb) return fallback();
  const img = element('img'); img.alt = '';
  setIcon(img, icon);
  if (orb) img.onerror = () => img.replaceWith(fallback());
  return img;
}
function renderOmens(context,id,rule) {
  const list = $(context === 'basic' ? 'orb-omens' : 'special-omens'); list.replaceChildren();
  const options = relevantOmens(omens,id,rule,data());
  // Kept omens survive a switch to a currency they do not fit, and work
  // again when it comes back.
  if (!keepOmens) activeOmens[context] = activeOmens[context].filter(id => options.some(([key]) => key === id));
  if (!options.length) return;
  list.append(element('strong',t('omen.next')));
  for (const [key,omen] of options) {
    const label = element('label'); const input = element('input'); input.type = 'checkbox';
    input.checked = activeOmens[context].includes(key); input.disabled = Boolean(item.reveal);
    input.setAttribute('aria-label',omen.name);
    const img = iconElement(omen.icon);
    const usage = payment(key,omen);
    label.append(input,img,element('span',omen.name),element('small',usage.unit_ex === null ? t('omen.noPrice') : `${num(usage.unit_ex,4)} Ex`));
    input.onchange = () => {
      if (input.checked) activeOmens[context] = [...activeOmens[context].filter(id => !omen.exclusives?.includes(id)),key];
      else activeOmens[context] = activeOmens[context].filter(id => id !== key);
      render(); persist(); status(t('omen.updated'));
    };
    list.append(label);
  }
  const keep = element('label', undefined, 'omen-keep'), box = element('input'); box.type = 'checkbox';
  box.checked = keepOmens; box.setAttribute('aria-label', t('omen.keep'));
  keep.append(box, element('span', t('omen.keep')));
  box.onchange = () => { keepOmens = box.checked; render(); persist(); status(t(keepOmens ? 'omen.keepOn' : 'omen.keepOff')); };
  list.append(keep);
}
// The ticked omens that fit this currency (kept ones may not).
const usableOmens = (context,id,rule) => {
  const fits = new Set(relevantOmens(omens,id,rule,data()).map(([key]) => key));
  return activeOmens[context].filter(key => fits.has(key));
};
const effectsFor = (context,id,rule) => omenEffects(omens,usableOmens(context,id,rule),id,rule,data());
const operationPayments = (context,id,rule) => [payment(id,rule),...usableOmens(context,id,rule).map(id => payment(id,omens[id]))];
function renderSpecials() {
  const context = mode === 'essence' ? 'essence' : 'desecrate';
  const visible = Object.entries(specials).filter(([id,r]) => applicable(r,data()) &&
    r.operation === context && (r.operation === 'desecrate' || (essenceTier(id,r) && essenceRows(data(),r).length)));
  if (!visible.some(([id]) => id === specialSelected)) specialSelected = visible[0]?.[0] || '';
  // Essences and bones are picked from cards that say what each one does.
  $('special-field').hidden = true;
  $('essence-grid').hidden = false;
  if (context === 'essence') renderEssences(visible); else renderBones(visible);
  $('special').replaceChildren();
  for (const op of ['desecrate','essence']) {
    const group = element('optgroup'); group.label = t(`special.group.${op}`);
    for (const [id,r] of visible.filter(([,r]) => r.operation === op)) group.append(new Option(r.name,id));
    $('special').append(group);
  }
  $('special').value = specialSelected;
  const rule = specials[specialSelected];
  if (!rule) {
    $('special-name').textContent = ''; $('special-detail').textContent = t('err.notForClass');
    $('special-apply').disabled = true; $('special-omens').replaceChildren();
    return;
  }
  renderOmens(context,specialSelected,rule);
  $('special').disabled = Boolean(item.reveal);
  const usage = payment(specialSelected,rule);
  $('special-price').disabled = Boolean(item.reveal);
  // Rates arrive with full float precision; two decimals are what one types.
  $('special-price').value = usage.unit_ex == null ? '' : Math.round(usage.unit_ex * 100) / 100;
  $('special-price').placeholder = t('special.priceUnknown');
  $('special-name').textContent = rule.name;
  setIcon($('special-icon'), rule.icon);
  const effects = effectsFor(context,specialSelected,rule);
  const reason = specialReason(item,data(),rule,effects);
  $('special-detail').textContent = reason || (rule.operation === 'desecrate'
    ? t('special.desecrate') + (item.mods.length === 6 ? t('special.desecrateFull') : '')
    : `${rule.removes ? t('special.essenceRemoves') : t('special.essenceAdds')} ${essenceRows(data(),rule).map(m => m.text).join(' / ')}`);
  $('special-apply').disabled = Boolean(reason);
  $('desecrate-note').hidden = context !== 'desecrate';
}

// One card per essence, grouped by how it works, with the modifier(s) it
// guarantees on this item class.
const essenceText = text => text.replace(/[—]/g, '–').replace(/<br\s*\/?\s*>/gi, '\n');
function renderEssences(visible) {
  const grid = $('essence-grid'); grid.replaceChildren();
  for (const tierName of ['greater', 'perfect', 'special']) {
    const entries = visible.filter(([id, r]) => essenceTier(id, r) === tierName);
    if (!entries.length) continue;
    const head = element('div', undefined, 'essence-head');
    head.append(element('h3', t(`essence.${tierName}`)), element('p', t(`essence.${tierName}.hint`)));
    grid.append(head);
    const list = element('div', undefined, 'essence-list');
    for (const [id, rule] of entries) {
      const reason = specialReason(item, data(), rule);
      const card = element('button', undefined, `essence-card${reason ? ' unavailable' : ''}`);
      card.type = 'button';
      card.setAttribute('aria-pressed', String(id === specialSelected));
      card.title = reason ? `${rule.name}: ${reason}` : rule.name;
      const top = element('div', undefined, 'essence-top');
      const usage = payment(id, rule);
      top.append(iconElement(rule.icon), element('strong', rule.name.replace(/^(Greater|Perfect) /, '')),
        element('small', usage.unit_ex === null ? '' : `${num(usage.unit_ex, 2)} Ex`));
      card.append(top);
      const shown = new Set();
      for (const row of essenceRows(data(), rule)) {
        // Variants that read the same (one per attribute) are shown once.
        if (shown.has(row.affix + row.text)) continue;
        shown.add(row.affix + row.text);
        const line = element('p', `${row.affix === 'Prefix' ? 'P' : 'S'} · ${essenceText(row.text)}`, 'essence-mod');
        if (row.required_ilvl > item.ilvl) line.append(element('em', ` · ilvl ${row.required_ilvl}`));
        card.append(line);
      }
      card.onclick = () => {
        if (held?.id === id) { drop(); return; }
        specialSelected = id; specialRemember.essence = id; hold(id, rule.icon); render(); persist();
        status(reason || t('currency.holdHint'));
      };
      list.append(card);
    }
    grid.append(list);
  }
}

// Bone cards: every bone adds one unrevealed Desecrated affix; the kinds
// differ by item level cap (Gnawed) and the lowest modifier level (Ancient).
function renderBones(visible) {
  const grid = $('essence-grid'); grid.replaceChildren();
  const head = element('div', undefined, 'essence-head');
  head.append(element('h3', t('bone.title')), element('p', t('bone.hint')));
  const list = element('div', undefined, 'essence-list');
  const rank = id => ['gnawed', 'preserved', 'ancient'].indexOf(id.split('-')[0]);
  for (const [id, rule] of [...visible].sort(([a], [b]) => rank(a) - rank(b))) {
    const reason = specialReason(item, data(), rule, effectsFor('desecrate', id, rule));
    const card = element('button', undefined, `essence-card${reason ? ' unavailable' : ''}`);
    card.type = 'button';
    card.setAttribute('aria-pressed', String(id === specialSelected));
    card.title = reason ? `${rule.name}: ${reason}` : rule.name;
    const top = element('div', undefined, 'essence-top'), usage = payment(id, rule);
    top.append(iconElement(rule.icon), element('strong', rule.name),
      element('small', usage.unit_ex === null ? '' : `${num(usage.unit_ex, 2)} Ex`));
    card.append(top, element('p', rule.maxItemLevel ? t('bone.gnawed', rule.maxItemLevel)
      : rule.minimum ? t('bone.ancient', rule.minimum) : t('bone.preserved'), 'essence-mod'));
    card.onclick = () => {
      if (held?.id === id) { drop(); return; }
      selectMode('desecrate', id); hold(id, rule.icon); status(reason || t('currency.holdHint'));
    };
    list.append(card);
  }
  grid.append(head, list);
}

// The Well of Souls: an unrevealed Desecrated modifier waits here until the
// player reveals it, in any mode. Omen of Abyssal Echoes is used up on the
// reveal and allows one reroll of the three options.
const echoId = 'omen-of-abyssal-echoes';
let echoChosen = false;
function renderReveal() {
  const box = $('reveal-choices'); box.replaceChildren();
  const index = unrevealedIndex(item);
  if (item.reveal) {
    box.append(element('h3', item.reveal.rerolled ? t('reveal.titleRerolled') : t('reveal.title')));
    item.reveal.choices.forEach((m,i) => {
      const button = element('button',`${m.affix === 'Prefix' ? 'P' : 'S'}${m.tier} · ${rolledText(m)}`);
      button.setAttribute('aria-label',t('reveal.option', i+1));
      button.onclick = () => commit(revealChoice(item,i),t('reveal.picked', m.name));
      box.append(button);
    });
    if (item.reveal.echo && !item.reveal.rerolled) {
      const reroll = element('button', t('reveal.reroll'), 'reroll');
      reroll.onclick = () => attempt(() => commit(rerollReveal(item,data()), t('reveal.rerolled')));
      box.append(reroll);
    }
    return;
  }
  if (index < 0) return;
  box.append(element('h3', t('reveal.waiting', item.mods[index].affix)), element('p', t('reveal.waitingHint'), 'hint'));
  const echo = rules[echoId];
  if (echo) {
    const label = element('label', undefined, 'echo-option'), input = element('input'); input.type = 'checkbox';
    input.checked = echoChosen; input.onchange = () => { echoChosen = input.checked; };
    const usage = payment(echoId, echo);
    label.append(input, iconElement(echo.icon), element('span', echo.name),
      element('small', usage.unit_ex === null ? t('omen.noPrice') : `${num(usage.unit_ex,4)} Ex`));
    box.append(label);
  }
  const open = element('button', t('reveal.open'), 'primary');
  open.onclick = () => attempt(() => {
    const echo = echoChosen && rules[echoId];
    commit(startReveal(item, data(), { echo: Boolean(echo) }), echo ? t('reveal.openedEcho') : t('reveal.opened'),
      echo ? payment(echoId, rules[echoId]) : null);
    echoChosen = false;
  });
  box.append(open);
}

function currencyRow(label) {
  const row = element('div', undefined, 'currency-row');
  row.append(element('span', label, 'currency-row-label'));
  const list = element('div', undefined, 'currency-row-list'); row.append(list);
  $('currencies').append(row);
  return list;
}

function renderCurrencies() {
  $('currencies').replaceChildren();
  const rows = { '': currencyRow(t('currency.standard')), II: currencyRow('Greater · II'), III: currencyRow('Perfect · III') };
  const jewellery = catalystClasses.includes(data().options?.ItemClassesCode);
  if (jewellery) rows.Catalyst = currencyRow(t('currency.catalyst'));
  const classCode = data().options?.ItemClassesCode;
  for (const [id, rule] of visibleRules()) {
    if (rule.tier === 'Catalyst' && !jewellery) continue;
    if (rule.tier === 'Vaal' && !rule.classes.includes(classCode)) continue;
    if (rule.tier === 'Vaal') rows.Vaal ??= currencyRow(t('currency.infuser'));
    const reason = currencyReason(item, data(), id, rule);
    const button = element('button', undefined, `currency${reason ? ' unavailable' : ''}`);
    button.dataset.currency = id; button.setAttribute('aria-pressed', String(mode === 'basic' && selected === id));
    button.setAttribute('aria-label', rule.name + (rule.tier ? ` ${rule.tier}` : ''));
    button.title = reason ? `${rule.name}: ${reason}` : rule.name;
    const img = iconElement(rule.icon, true);
    button.append(img, element('span', shortNames[kind(id)] || rule.short || rule.name));
    button.onclick = () => {
      if (held?.id === id) { drop(); return; }
      selected = id; selectMode('basic'); hold(id, rule.icon); status(reason || t('currency.holdHint'));
    };
    rows[rule.tier || ''].append(button);
  }
  const other = currencyRow(t('currency.other'));
  const bones = Object.entries(specials).filter(([,r]) => r.operation === 'desecrate' && applicable(r,data()));
  bones.sort(([a],[b]) => ['gnawed','preserved','ancient'].indexOf(a.split('-')[0]) - ['gnawed','preserved','ancient'].indexOf(b.split('-')[0]));
  for (const [id,rule] of bones) {
    const reason = specialReason(item,data(),rule,effectsFor('desecrate',id,rule));
    const button = element('button',undefined,`currency${reason ? ' unavailable' : ''}`);
    button.setAttribute('aria-label',rule.name);
    button.setAttribute('aria-pressed',String(mode === 'desecrate' && specialSelected === id));
    button.title = reason ? `${rule.name}: ${reason}` : rule.name;
    const img = iconElement(rule.icon, true);
    button.append(img,element('span',rule.name));
    button.onclick = () => {
      if (held?.id === id) { drop(); return; }
      selectMode('desecrate',id); hold(id, rule.icon); status(reason || t('currency.holdHint'));
    };
    other.append(button);
  }
  // Not every class has an essence (or the remembered one); show the button
  // with the first essence that fits this class.
  const essences = Object.entries(specials).filter(([id,r]) => essenceTier(id,r) && applicable(r,data()) && essenceRows(data(),r).length);
  if (essences.length) {
    const essenceRule = specials[specialRemember.essence] && essences.some(([id]) => id === specialRemember.essence)
      ? specials[specialRemember.essence] : essences[0][1];
    const essenceButton = element('button',undefined,'currency');
    essenceButton.setAttribute('aria-label','Essence');
    essenceButton.setAttribute('aria-pressed',String(mode === 'essence'));
    essenceButton.title = t('essence.pick');
    essenceButton.append(iconElement(essenceRule.icon, true),element('span','Essence'));
    essenceButton.onclick = () => { drop(); selectMode('essence'); status(t('essence.pickHint')); };
    other.append(essenceButton);
  }
  const rule = rules[selected];
  renderOmens('basic',selected,rule);
  const omenEffect = effectsFor('basic',selected,rule);
  const reason = orbOmenReason(item,data(),selected,rule,omenEffect);
  $('selected-name').textContent = rule.name + (rule.tier ? ` · ${rule.tier}` : '');
  $('selected-detail').textContent = reason || `${t(`effect.${rule.afterTrigger}`)}${rule.afterRarity && rule.afterRarity !== item.rarity ? t('effect.becomes', rule.afterRarity) : ''}${rule.beforeMin_mod_lv ? t('effect.minLevel', rule.beforeMin_mod_lv) : ''}`;
  $('apply').disabled = Boolean(reason);
  if (selected === 'vaal-orb') {
    const why = corruptReason(item), outcomes = corruptOutcomes(item, data(), corruptContext());
    $('selected-detail').textContent = why || t('vaal.detail', outcomes.map(o => t(`vaal.o.${o}`)).join(' · '), num(100 / outcomes.length, 1));
    $('apply').disabled = Boolean(why);
  }
  if (!reason && rule.afterTrigger === 'infuse') {
    const q = currentQuality(item, data()), max = maxQuality(item);
    $('selected-detail').textContent = t('effect.infuseOdds', q, max, max + infuserBeyond, num(corruptChance(q, max) * 100, 0));
  }
  if (!reason && rule.afterTrigger === 'fracture') $('selected-detail').textContent = t('effect.fractureOdds', num(100 / fracturable(item).length, 2));
  if (!reason && usableOmens('basic',selected,rule).length) {
    const removal = omenEffect.removal || {};
    $('selected-detail').textContent = omenEffect.sanctify ? t('effect.sanctify') : selected.includes('exalted')
      ? t('effect.omens', omenEffect.quantity, omenEffect.side ? t('effect.side', omenEffect.side) : '') +
        (omenEffect.catalyse && item.catalyst?.quality ? t('effect.catalyse', catalystTypes[item.catalyst.id]?.label || '?', item.catalyst.quality, num(1 + 0.2 * Math.min(item.catalyst.quality, 20), 1)) : '')
      : t('effect.removes', removal.count || 1) + (removal.side ? t('effect.side', removal.side) : '') +
        (removal.desecrated ? t('effect.onlyDesecrated') : '') + (removal.lowest ? t('effect.lowest') : '') +
        (rule.afterTrigger === 'del_add' ? t('effect.thenAdds') : '') + t('effect.omensUsed');
  }
}

function renderPools() {
  const present = new Set(data().mods.map(m => m.pool));
  const options = [...basePools.filter(p => present.has(p)).map(p => [p, p === 'normal' ? t('pool.normal') : p === 'perfect_essence' ? 'Perfect Essence' : p[0].toUpperCase() + p.slice(1)]),
    ...pageRunes().filter(([, r]) => present.has(r.pool)).map(([, r]) => [r.pool, `${r.name} · ${r.label}`])];
  if (!options.some(([p]) => p === pool)) pool = 'normal';
  $('pool').replaceChildren(...options.map(([value, label]) => new Option(label, value)));
  $('pool').value = pool;
  const rune = pageRunes().find(([, r]) => r.pool === pool)?.[1];
  $('pool-hint').hidden = !rune;
  if (rune) $('pool-hint').textContent = rollPools(item).includes(pool) ? t('pool.runeActive', rune.name) : t('pool.runeInactive', rune.name);
}

// What the list's percentages say: with an orb or bone that adds a modifier
// selected, the chance one use puts each row on this item (odds.mjs);
// otherwise, or when it cannot be used now, each row's share of its side.
function listOdds() {
  try {
    if (mode === 'basic') {
      const rule = rules[selected];
      if (!rule || !['add', 'del_add'].includes(rule.afterTrigger)) return null;
      const effects = effectsFor('basic', selected, rule), reason = orbOmenReason(item, data(), selected, rule, effects);
      if (reason) return { name: rule.name, reason };
      const two = selected.includes('exalted') && effects.quantity === 2;
      return { name: rule.name, odds: orbOdds(item, data(), selected, rule, effects),
        kind: two ? 'two' : rule.afterTrigger, minimum: rule.beforeMin_mod_lv };
    }
    if (mode === 'desecrate') {
      if (unrevealedIndex(item) >= 0) return { odds: revealOdds(item, data()), kind: 'reveal' };
      const rule = specials[specialSelected];
      if (!rule) return null;
      const effects = effectsFor('desecrate', specialSelected, rule), reason = specialReason(item, data(), rule, effects);
      if (reason) return { name: rule.name, reason };
      return { name: rule.name, odds: desecrateOdds(item, data(), rule, effects), kind: 'offer', minimum: rule.minimum };
    }
  } catch { /* conflicting omens: the plain shares */ }
  return null;
}
const pct = p => `${num(p * 100, p < 0.001 ? 4 : 3)}%`;

function renderMods() {
  renderPools();
  const chance = listOdds();
  $('odds-note').textContent = !chance ? t('odds.share') : chance.reason ? t('odds.cannot', chance.name, chance.reason)
    : t(`odds.${chance.kind}`, chance.name) + (chance.minimum > 1 ? t('odds.minimum', chance.minimum) : '');
  const open = new Set([...document.querySelectorAll('details[open]')].map(n => n.dataset.family));
  const search = $('search').value.trim().toLowerCase();
  let listEffects = {side:null,tags:[],quantity:1};
  if (mode === 'desecrate' && specials[specialSelected]) listEffects = effectsFor('desecrate',specialSelected,specials[specialSelected]);
  if (mode === 'desecrate' && item.reveal?.effects) listEffects = item.reveal.effects;
  if (mode === 'basic') listEffects = effectsFor('basic',selected,rules[selected]);
  const rows = filterOmenRows(data().mods.filter(m => m.pool === pool && ['Prefix', 'Suffix'].includes(m.affix) &&
    (!['essence','perfect_essence'].includes(pool) || Object.values(specials).some(r => r.operation === 'essence' &&
      essenceRows(data(),r).some(row => row.source_id === m.source_id && row.pool === m.pool)))),listEffects);
  // The base pool and a socketed rune's pool roll together, so their odds
  // are shares of the combined weight; a rune pool without its rune cannot roll.
  const pools = rollPools(item).includes(pool) ? rollPools(item) : [pool];
  let potential = Object.values(runes).some(r => r.pool === pool) && !rollPools(item).includes(pool) ? []
    : filterOmenRows(candidates(item, data(), { pool: pools, rarity: 'Rare' }),listEffects);
  if (listEffects.catalyse) potential = catalystBoost(item, potential);
  for (const side of ['Prefix', 'Suffix']) {
    const list = $(side.toLowerCase() + '-list'); list.replaceChildren();
    const groups = new Map();
    for (const row of rows.filter(m => m.affix === side)) {
      const key = row.families[0];
      if (!groups.has(key)) groups.set(key, []);
      groups.get(key).push(row);
    }
    const sidePool = potential.filter(m => m.affix === side);
    const total = sidePool.reduce((sum, m) => sum + m.weight, 0);
    const sideChance = chance?.odds && data().mods.filter(m => m.affix === side).reduce((sum, m) => sum + (chance.odds.get(m.source_id) || 0), 0);
    $(side.toLowerCase() + '-total').textContent = !chance?.odds ? t('weight', num(total))
      : ['add', 'del_add'].includes(chance.kind) ? t('odds.side', pct(sideChance)) : '';
    for (const [family, mods] of groups) {
      if (search && !mods.some(m => `${m.text} ${m.name} ${m.tags.join(' ')} ${family}`.toLowerCase().includes(search))) continue;
      const details = element('details', undefined, 'family');
      const existingIndex = item.mods.findIndex(m => m.affix === side && m.families[0] === family);
      const existing = item.mods[existingIndex];
      details.dataset.family = `${pool}:${side}:${family}`;
      details.open = open.has(details.dataset.family) || Boolean(search);
      const summary = element('summary');
      const title = mods.at(-1).text.replace(/\((-?\d+(?:\.\d+)?)[—–](-?\d+(?:\.\d+)?)\)/g, '#');
      const visibleTier = existing && mods.find(m => m.source_id === existing.source_id || m.text === existing.text)?.tier;
      const familyChance = chance?.odds && mods.reduce((sum, m) => sum + (chance.odds.get(m.source_id) || 0), 0);
      summary.append(element('span', title, 'family-name'), element('span', (existing ?
        t('family.onItem', visibleTier ? `T${visibleTier}` : existing.name) : t('family.tiers', mods.length)) +
        (familyChance ? ` · ${pct(familyChance)}` : ''), 'family-count'));
      if (existing) details.classList.add('has-selected');
      const familyList = element('div', undefined, 'family-list');
      for (const row of mods.sort((a, b) => a.tier - b.tier)) {
        const active = existing && (existing.source_id === row.source_id || existing.text === row.text);
        const entry = element('div', undefined, `tier-row${active ? ' selected-tier' : ''}`);
        const description = element('div', undefined, 'tier-description');
        description.append(element('p', row.text));
        const eligible = potential.some(m => m.source_id === row.source_id);
        const weight = potential.find(m => m.source_id === row.source_id)?.weight ?? row.weight;
        const share = chance?.odds ? (chance.odds.get(row.source_id) ? pct(chance.odds.get(row.source_id)) : '—')
          : eligible && total ? `${num(weight / total * 100, 3)}%` : '—';
        description.append(element('small', `${row.name} · ilvl ${row.required_ilvl} · w ${row.weight} · ${share}`));
        const button = element('button', active ? '✓' : existing ? '↔' : '+');
        const reason = manualReason(existing ? removeMod(item, existingIndex) : item, row);
        button.disabled = active || Boolean(reason);
        button.title = active ? t('tier.onItem') : reason || (existing ? t('tier.replace') : t('tier.add'));
        button.setAttribute('aria-label', t('tier.aria', row.name, row.tier, t(active ? 'tier.aria.on' : existing ? 'tier.aria.replace' : 'tier.aria.add')));
        button.onclick = () => attempt(() => commit(existing ? replaceTier(item, existingIndex, row) : manualAdd(item, row),
          existing ? t('tier.replaced', existing.name, existing.tier, row.name, row.tier) : t('tier.added', row.name, row.tier)));
        entry.append(element('span', `T${row.tier}`, 'tier-id'), description, button);
        // ◎ makes this line and tier the Chaos simulation's target; only rows a
        // Chaos Orb can roll (the base pool and socketed rune pools) have it.
        if (rollPools(item).includes(row.pool)) {
          const aim = simTarget && lineKey(row) === simTarget.key && row.tier === simTarget.tier;
          const goal = element('button', '◎', `sim-aim${aim ? ' active' : ''}`);
          goal.title = t('sim.setTarget'); goal.setAttribute('aria-label', t('sim.setTargetAria', row.name, row.tier));
          goal.setAttribute('aria-pressed', String(Boolean(aim)));
          goal.onclick = () => setSimTarget(row);
          entry.classList.add('with-target'); entry.append(goal);
        }
        familyList.append(entry);
      }
      details.append(summary, familyList); list.append(details);
    }
    if (!list.children.length) list.append(element('p', t('pool.empty'), 'empty-pool'));
  }
}

function render() { if (ready) {
  if (held && mode === 'essence' && held.id !== specialSelected) drop();
  $('basic-controls').hidden = mode !== 'basic'; $('special-controls').hidden = mode === 'basic';
  renderItem(); renderSpecials(); renderReveal(); renderCurrencies(); renderMods(); renderSim(); renderLibrary(); markTarget();
} }

// Switching class or defence type starts a new item, like the old base picker.
async function switchPage(page, baseName) {
  if (!pages[page] || page === item.base) return;
  try {
    await load(page);
    commit({ ...createItem(page), ilvl: item.ilvl, quality: jewelleryPages.includes(item.base) ? undefined : qualityOf(item), baseName }, t('class.changed'), null, true);
  } catch (error) { status(error.message, 'error'); renderItem(); }
}
$('item-class').onchange = () => {
  const cls = classes.find(c => c.id === $('item-class').value);
  if (!cls) return;
  // Keep the defence type when the new class has it (Armour gloves → Armour boots).
  const attr = pages[item.base]?.attr;
  void switchPage((cls.variants.find(v => v.attr === attr) || cls.variants[0]).page);
};
$('variant').onchange = () => void switchPage($('variant').value);
function setRune(index, id) {
  attempt(() => {
    const list = runesOf(item), rune = runes[id], current = runes[list[index]];
    if (current?.bound || (id && list.includes(id)) || (!rune && !current)) { renderItem(); return; }
    list[index] = rune ? id : '';
    // Crafted modifiers already on the item stay when Astrid's goes.
    commit(withRunes({ ...item, sockets: socketsOf(item) }, list), rune ? t('rune.socketed', rune.name) : t('rune.swapped', current.name),
      rune ? payment(id, rune) : null);
  });
}
// A socket-bound rune keeps its socket: the count cannot go below one.
$('sockets').onchange = () => attempt(() => {
  const value = Number($('sockets').value);
  const list = runesOf(item);
  if (list.slice(value).some(Boolean)) { $('sockets').value = String(socketsOf(item)); throw new Error(t('sockets.runeBound')); }
  commit(withRunes({ ...item, sockets: value }, list.slice(0, value)), t('sockets.set', value));
});
// The base picker searches every defence type of the class. A base of the
// same page changes only the numbers (the modifier pool is the page's); one
// of another defence type switches the page, which starts a new craft.
let pickerRows = [], pickerActive = 0;
const baseSummary = b => [
  b.ar && `AR ${b.ar}`, b.ev && `EV ${b.ev}`, b.es && `ES ${b.es}`, b.block && `Block ${b.block}%`,
  b.phys && `${b.phys[0]}-${b.phys[1]} · ${b.aps.toFixed(2)}/s`,
].filter(Boolean).join(' · ');
function highlight(text, query) {
  const span = element('b');
  const at = query ? text.toLowerCase().indexOf(query) : -1;
  if (at < 0) { span.textContent = text; return span; }
  span.append(text.slice(0, at), element('mark', text.slice(at, at + query.length)), text.slice(at + query.length));
  return span;
}
function renderBasePicker() {
  const query = $('base-search').value.trim().toLowerCase();
  const variants = pages[item.base]?.cls.variants || [];
  // The current defence type first, then the others in the class's order.
  const ordered = [...variants.filter(v => v.page === item.base), ...variants.filter(v => v.page !== item.base)];
  const list = $('base-list'); list.replaceChildren(); pickerRows = [];
  const current = currentBase();
  for (const v of ordered) {
    const matches = [...baseList(v.page)].reverse().filter(b => !query || b.name.toLowerCase().includes(query));
    if (!matches.length) continue;
    if (variants.length > 1) list.append(element('li', variantName(v.attr), 'group'));
    for (const b of matches) {
      const li = element('li'); li.setAttribute('role', 'option'); li.id = `base-option-${pickerRows.length}`;
      li.setAttribute('aria-selected', String(v.page === item.base && b.name === current?.name));
      li.append(highlight(b.name, query), element('em', b.level));
      const summary = baseSummary(b); if (summary) li.append(element('small', summary));
      const index = pickerRows.length;
      li.onmousemove = () => { if (pickerActive !== index) { pickerActive = index; markActive(); } };
      li.onmousedown = event => event.preventDefault();
      li.onclick = () => pickBase(index);
      pickerRows.push({ page: v.page, base: b, node: li }); list.append(li);
    }
  }
  if (!pickerRows.length) list.append(element('li', t('base.none'), 'none'));
  const selectedIndex = pickerRows.findIndex(r => r.node.getAttribute('aria-selected') === 'true');
  pickerActive = query ? 0 : Math.max(0, selectedIndex);
  markActive();
}
function markActive() {
  pickerRows.forEach((r, i) => r.node.classList.toggle('active', i === pickerActive));
  const node = pickerRows[pickerActive]?.node;
  if (node) { node.scrollIntoView({ block: 'nearest' }); $('base-search').setAttribute('aria-activedescendant', node.id); }
}
function openBasePicker() {
  if ($('base').disabled) return;
  $('base-pop').hidden = false; $('base').setAttribute('aria-expanded', 'true');
  $('base-search').value = ''; renderBasePicker(); $('base-search').focus();
}
function closeBasePicker(focusButton = false) {
  if ($('base-pop').hidden) return;
  $('base-pop').hidden = true; $('base').setAttribute('aria-expanded', 'false');
  if (focusButton) $('base').focus();
}
function pickBase(index) {
  const row = pickerRows[index];
  if (!row) return;
  closeBasePicker(true);
  if (row.page === item.base) {
    if (row.base.name !== currentBase()?.name) attempt(() => commit({ ...item, baseName: row.base.name }, t('base.set', row.base.name)));
  } else void switchPage(row.page, row.base.name);
}
$('base').onclick = () => $('base-pop').hidden ? openBasePicker() : closeBasePicker();
$('base').onkeydown = event => { if (['ArrowDown', 'ArrowUp'].includes(event.key)) { event.preventDefault(); openBasePicker(); } };
$('base-search').oninput = renderBasePicker;
$('base-search').onkeydown = event => {
  if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
    event.preventDefault();
    if (pickerRows.length) { pickerActive = (pickerActive + (event.key === 'ArrowDown' ? 1 : -1) + pickerRows.length) % pickerRows.length; markActive(); }
  } else if (event.key === 'Enter') { event.preventDefault(); pickBase(pickerActive); }
  else if (event.key === 'Escape') { event.preventDefault(); closeBasePicker(true); }
  else if (event.key === 'Tab') closeBasePicker();
};
document.addEventListener('mousedown', event => { if (!event.target.closest('.base-picker')) closeBasePicker(); });
$('quality').onchange = () => attempt(() => {
  const value = Number($('quality').value);
  if (!Number.isInteger(value) || value < 0 || value > maxQuality(item) + infuserBeyond) { $('quality').value = qualityOf(item); throw new Error(t('quality.invalid', maxQuality(item) + infuserBeyond)); }
  commit({ ...item, quality: value }, t('quality.set', value));
});
$('rarity').onchange = () => attempt(() => {
  const value = $('rarity').value;
  try { commit(setRarity(item, value), t('rarity.set', value)); } finally { $('rarity').value = item.rarity; }
});
$('ilvl').onchange = () => attempt(() => {
  const value = Number($('ilvl').value);
  if (!Number.isInteger(value) || value < 1 || value > 100) { $('ilvl').value = item.ilvl; throw new Error(t('ilvl.invalid')); }
  if (item.mods.some(m => m.required_ilvl > value)) { $('ilvl').value = item.ilvl; throw new Error(t('ilvl.tooLow')); }
  commit({ ...item, ilvl: value }, t('ilvl.set', value));
});
$('clear').onclick = () => commit(clearMods(item), t('cleared', item.rarity));
$('reset').onclick = () => commit({ ...createItem(item.base), ilvl: item.ilvl, baseName: item.baseName, quality: qualityOf(item), sockets: socketsOf(item) }, t('reset.done'),null,true);
$('undo').onclick = () => {
  if (simBusy || simPending) return;
  const last = undo.pop(); if (!last) return;
  item = last.item; history = last.history; sessionStart = last.sessionStart; activeOmens = last.activeOmens;
  render(); status(t('undo.done')); persist();
};
$('special').onchange = () => { specialSelected = $('special').value; specialRemember[mode] = specialSelected; render(); persist(); };
function selectMode(context,id) {
  mode = context;
  if (mode !== 'basic') {
    specialSelected = id || specialRemember[mode];
    specialRemember[mode] = specialSelected;
  }
  pool = mode === 'desecrate' ? 'desecrated' : mode === 'essence' ? 'essence' : 'normal';
  $('pool').value = pool; render(); persist();
}
$('special-price').onchange = () => attempt(() => {
  const value = Number($('special-price').value);
  if ($('special-price').value === '') delete priceOverrides[specialSelected];
  else if (!Number.isFinite(value) || value < 0) throw new Error(t('price.invalid'));
  else priceOverrides[specialSelected] = value;
  renderSpecials(); persist(); status(t('price.override'));
});
$('special-apply').onclick = () => attempt(() => {
  const rule = specials[specialSelected];
  const context = rule.operation;
  const effects = effectsFor(context,specialSelected,rule), payments = operationPayments(context,specialSelected,rule);
  commit(applySpecial(item,data(),rule,Math.random,effects),t('used', payments.map(p=>p.name).join(' + ')),payments);
});
$('pool').onchange = () => { pool = $('pool').value; renderMods(); };
$('search').oninput = renderMods;
// What a Vaal Orb needs to know of the item besides its modifiers.
const corruptContext = () => ({ classId: classOf(item.base)?.id, sockets: socketsOf(item), maxSockets: maxSockets(item.base), quality: qualityOf(item) });
$('apply').onclick = () => attempt(() => {
  const rule = rules[selected];
  if (selected === 'vaal-orb') {
    const result = corrupt(item, data(), corruptContext());
    commit(result.item, t(`vaal.done.${result.outcome}`, result.detail), [payment(selected, rule)]);
    return;
  }
  const effects = effectsFor('basic',selected,rule), payments = operationPayments('basic',selected,rule);
  commit(applyOrbOmens(item,data(),selected,rule,effects), t('applied', payments.map(p=>p.name).join(' + ')),payments);
});
// Holding a currency, as in game: the cursor becomes the orb, a click on the
// item uses it, and it stays in hand (like Shift-clicking) until a right
// click, Esc or a click on the same orb again.
let held = null;
const ghost = element('div', undefined, 'held-orb'); ghost.hidden = true;
document.body.append(ghost);
function hold(id, icon) {
  held = { id, icon };
  ghost.replaceChildren(iconElement(icon, true)); ghost.hidden = false;
  document.body.classList.add('holding');
  markTarget();
}
function drop() {
  if (!held) return;
  held = null; ghost.hidden = true;
  document.body.classList.remove('holding');
  markTarget();
}
// The button the held currency would press, and whether it can now.
function heldAction() {
  if (!held) return null;
  const button = mode === 'basic' ? $('apply') : $('special-apply');
  const detail = mode === 'basic' ? $('selected-detail') : $('special-detail');
  return { button, ok: !button.disabled && !item.reveal, reason: item.reveal ? t('err.pendingReveal') : detail.textContent };
}
function markTarget() {
  const action = heldAction();
  $('item-card').classList.toggle('target-ok', Boolean(action?.ok));
  $('item-card').classList.toggle('target-bad', Boolean(action && !action.ok));
  $('item-card').title = action && !action.ok ? action.reason : '';
  markRemoval(action);
}
// Over the item with an orb that removes (Chaos, Annulment), the affixes it
// may take are marked with their odds and the others dimmed; the omens count
// (Whittling: only the lowest modifier level, Erasure: one side, Light:
// only Desecrated).
let overCard = false;
function markRemoval(action) {
  const rows = [...$('item-mods').querySelectorAll('.item-mod')];
  for (const row of rows) {
    row.classList.remove('may-remove', 'kept');
    row.querySelector('.remove-odds')?.remove();
  }
  const rule = rules[selected];
  if (!overCard || !action?.ok || mode !== 'basic' || !rule || !['del', 'del_add'].includes(rule.afterTrigger)) return;
  let removal;
  try { removal = effectsFor('basic', selected, rule).removal || {}; } catch { return; }
  const options = removable(item, removal);
  if (!options.length) return;
  const share = Math.min(1, (removal.count || 1) / options.length);
  for (const row of rows) {
    const hit = options.includes(Number(row.dataset.index));
    row.classList.add(hit ? 'may-remove' : 'kept');
    if (hit) {
      const odds = element('span', t('remove.odds', num(share * 100, share < 1 ? 1 : 0)), 'remove-odds');
      odds.title = t('remove.oddsHint');
      row.querySelector('.mod-side').append(odds);
    }
  }
}
$('item-card').addEventListener('mouseenter', () => { overCard = true; markTarget(); });
$('item-card').addEventListener('mouseleave', () => { overCard = false; markTarget(); });
document.addEventListener('mousemove', event => {
  if (held) ghost.style.transform = `translate(${event.clientX}px, ${event.clientY}px)`;
});
document.addEventListener('contextmenu', event => { if (held) { event.preventDefault(); drop(); status(t('currency.dropped')); } });
document.addEventListener('keydown', event => { if (event.key === 'Escape' && held && $('base-pop').hidden) { drop(); status(t('currency.dropped')); } });
// F11 inside the craft page sizes the app window (the host page owns it).
document.addEventListener('keydown', event => { if (event.key === 'F11') { event.preventDefault(); parent.postMessage({ type: 'craft-size' }, location.origin); } });
$('item-card').addEventListener('click', event => {
  const action = heldAction();
  if (!action || event.target.closest('button, input, select, label')) return;
  if (!action.ok) { status(action.reason, 'error'); return; }
  action.button.click();
});
// The price check's ⚒ button sends the item the player copied in game.
async function importCopied(copied) {
  const where = pageFor(copied, { classes, bases });
  if (where.error) { status(where.error === 'unique' ? t('import.unique') : t('import.class', copied.class || '?'), 'error'); return; }
  try {
    const { item: next, unmatched } = importItem(copied, where.page, await load(where.page));
    if (baseList(where.page).some(b => b.name === copied.baseType)) next.baseName = copied.baseType;
    const found = runesFor(copied.raw, where.page, runes);
    next.runeStats = runeStatLines(copied.raw, runes);
    const socketLine = /^Sockets: (.+)$/m.exec(copied.raw);
    const sockets = Math.min(maxSockets(where.page), Math.max(found.length, socketLine ? socketLine[1].trim().split(/\s+/).length : 0));
    Object.assign(next, withRunes({ ...next, sockets }, [...found, ...Array(Math.max(0, sockets - found.length)).fill('')].slice(0, sockets)));
    const quality = /^Quality: \+(\d+)%/m.exec(copied.raw);
    next.quality = quality ? Math.min(maxQuality(next) + infuserBeyond, Number(quality[1])) : 0;
    const catalyst = catalystFromCopy(copied.raw);
    if (catalyst) next.catalyst = catalyst;
    // A copied Corrupted or Sanctified item stays locked in the craft too.
    if (/^Sanctified$/m.test(copied.raw)) next.sanctified = true;
    else if (/^Corrupted$/m.test(copied.raw)) next.corrupted = true;
    mode = 'basic'; pool = 'normal'; $('pool').value = pool;
    commit(next, t('import.done', itemLabel(where.page), next.mods.length) +
      (unmatched.length ? t('import.unmatched', unmatched.length, unmatched.join(', ')) : '') +
      (where.guessed ? t('import.guessed') : ''), null, true);
    if (unmatched.length || where.guessed) status($('status').textContent, 'error');
  } catch (error) { status(error.message, 'error'); }
}

// ---- Chaos simulation --------------------------------------------------
// The user marks a modifier line and tier (◎ in the list below), picks a
// Chaos Orb and its omens; the simulation presses that orb on the current
// item until the line reaches the tier. "Simulate" repeats it many times for
// the statistics; "Watch" plays one run on the item card, orb by orb.
const simCap = 20000;
let simTarget = null, simOrb = 'chaos', simOmens = [], simResult = null, simBusy = '', simStop = false, simPending = null;
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));
const chaosIds = () => visibleRules().map(([id]) => id).filter(id => rules[id].afterTrigger === 'del_add' && id.includes('chaos'));
const lineText = text => text.replace(/\((-?\d+(?:\.\d+)?)[—–](-?\d+(?:\.\d+)?)\)|-?\d+(?:\.\d+)?/g, '#');
const simLines = target => {
  const seen = new Set();
  return data().mods.filter(m => rollPools(item).includes(m.pool) && lineKey(m) === target.key)
    .sort((a, b) => a.tier - b.tier).filter(m => !seen.has(m.tier) && seen.add(m.tier));
};
const simEffects = () => omenEffects(omens, simOmens, simOrb, rules[simOrb], data());
const simSignature = () => JSON.stringify([snapshot(item), simTarget, simOrb, simOmens]);
// The cost of one press: the orb and every omen it uses up; null when one
// of them has no price.
function simUnit() {
  const parts = [payment(simOrb, rules[simOrb]), ...simOmens.map(id => payment(id, omens[id]))];
  return parts.some(p => p.unit_ex === null) ? null : parts.reduce((sum, p) => sum + p.unit_ex, 0);
}
function simReason() {
  try { return targetReason(item, data(), simOrb, rules[simOrb], simEffects(), simTarget); }
  catch (error) { return error.message; }
}
function setSimTarget(row) {
  simTarget = targetOf(row); simResult = null;
  persist(); render(); status(t('sim.targetSet', lineText(row.text), row.tier), 'success');
}

function renderSim() {
  const ids = chaosIds();
  if (!ids.includes(simOrb)) simOrb = ids[0] || 'chaos';
  $('sim-orb').replaceChildren(...ids.map(id => new Option(rules[id].name, id)));
  $('sim-orb').value = simOrb;
  const lines = simTarget ? simLines(simTarget) : [];
  if (simTarget && !lines.length) simTarget = null;
  const box = $('sim-target'); box.replaceChildren();
  if (!simTarget) box.append(element('p', t('sim.pickTarget'), 'hint'));
  else {
    const select = element('select'); select.id = 'sim-tier'; select.setAttribute('aria-label', t('sim.tier'));
    for (const row of lines) select.append(new Option(t('sim.tierOption', row.tier, row.text, row.required_ilvl), row.tier));
    select.value = String(simTarget.tier); select.disabled = Boolean(simBusy || simPending);
    select.onchange = () => { simTarget = { ...simTarget, tier: Number(select.value) }; persist(); render(); };
    const clear = element('button', '×'); clear.type = 'button'; clear.setAttribute('aria-label', t('sim.clearTarget'));
    clear.disabled = Boolean(simBusy || simPending);
    clear.onclick = () => { simTarget = null; simResult = null; persist(); render(); };
    box.append(element('span', `${simTarget.affix} · ${lineText(simTarget.text)}`, 'sim-line'), select, clear);
  }
  const list = $('sim-omens'); list.replaceChildren();
  const options = relevantOmens(omens, simOrb, rules[simOrb], data());
  simOmens = simOmens.filter(id => options.some(([key]) => key === id));
  for (const [key, omen] of options) {
    const label = element('label'), input = element('input'); input.type = 'checkbox';
    input.checked = simOmens.includes(key); input.disabled = Boolean(simBusy || simPending);
    input.setAttribute('aria-label', omen.name);
    const usage = payment(key, omen);
    label.append(input, iconElement(omen.icon), element('span', omen.name),
      element('small', usage.unit_ex === null ? t('omen.noPrice') : `${num(usage.unit_ex, 4)} Ex`));
    input.onchange = () => {
      simOmens = input.checked ? [...simOmens.filter(id => !omen.exclusives?.includes(id)), key] : simOmens.filter(id => id !== key);
      persist(); render();
    };
    list.append(label);
  }
  if (options.length) list.prepend(element('strong', t('sim.omens')));
  const reason = simBusy || simPending ? '' : simReason();
  // Without a target the hint above says what to do; no red line for it.
  $('sim-reason').textContent = simTarget ? reason : '';
  $('sim-run').disabled = $('sim-watch').disabled = Boolean(reason || simBusy || simPending);
  $('sim-orb').disabled = $('sim-runs').disabled = Boolean(simBusy || simPending);
  $('sim-stop').hidden = !simBusy;
  $('sim-pending').hidden = !simPending;
  $('workbench').classList.toggle('sim-running', Boolean(simBusy || simPending));
  renderSimResult();
}

function renderSimResult() {
  const box = $('sim-result');
  box.hidden = !simResult;
  if (!simResult) return;
  box.replaceChildren();
  const r = simResult, s = r.stats, div = divineEx();
  const money = n => r.unit === null ? '' : ` · ${exText(n * r.unit)} Ex${div ? ` (${num(n * r.unit / div, 2)} Div)` : ''}`;
  box.append(element('p', t('sim.summary', num(r.done, 0), r.orbName), 'sim-head'));
  if (s) {
    const grid = element('div', undefined, 'sim-stats');
    for (const [key, value] of [['sim.mean', s.mean], ['sim.median', s.median], ['sim.p90', s.p90], ['sim.best', s.min], ['sim.worst', s.max]]) {
      const cell = element('div');
      cell.append(element('small', t(key)), element('b', num(Math.round(value), 0)), element('span', money(value).replace(/^ · /, ''), 'sim-money'));
      grid.append(cell);
    }
    box.append(grid);
    const chances = element('ul', undefined, 'sim-chances');
    for (const [pct, value] of [[25, s.p25], [50, s.median], [75, s.p75], [90, s.p90], [99, s.p99]]) {
      chances.append(element('li', t('sim.within', pct, num(value, 0)) + money(value)));
    }
    box.append(chances);
    const max = Math.max(...r.bars.map(b => b.count), 1), chart = element('div', undefined, 'sim-chart');
    chart.setAttribute('role', 'img'); chart.setAttribute('aria-label', t('sim.chart'));
    for (const bar of r.bars) {
      const column = element('span'); column.style.height = `${Math.max(2, bar.count / max * 100)}%`;
      column.title = t('sim.bar', num(bar.from, 0), num(bar.to, 0), num(bar.count, 0));
      chart.append(column);
    }
    box.append(chart, element('p', t('sim.axis', num(r.bars.at(-1)?.to ?? 0, 0)), 'hint'));
  }
  if (r.capped) box.append(element('p', t('sim.capped', num(r.capped, 0), num(r.capped / r.done * 100, 1), num(simCap, 0)), 'sim-warn'));
  if (r.stuck) box.append(element('p', t('sim.stuck', num(r.stuck, 0)), 'sim-warn'));
  if (r.early) box.append(element('p', t('sim.early'), 'sim-warn'));
  if (r.unit === null) box.append(element('p', t('sim.noPrice'), 'hint'));
  if (r.signature !== simSignature()) box.append(element('p', t('sim.stale'), 'hint'));
}

async function runSim() {
  if (simBusy || simReason()) return;
  const rule = rules[simOrb], effects = simEffects(), target = simTarget, total = Number($('sim-runs').value) || 10000;
  const runner = fastRunner(item, data(), rule, effects, target);
  const counts = [];
  let done = 0, capped = 0, stuck = 0, early = false;
  simBusy = 'run'; simStop = false; render();
  const signature = simSignature(), unit = simUnit(), orbName = rule.name + (simOmens.length ? ' + ' + simOmens.map(id => omens[id].name).join(' + ') : '');
  while (done < total && !simStop) {
    const start = performance.now();
    while (done < total && performance.now() - start < 40) {
      const result = runner(Math.random, simCap);
      if (result.capped) capped++; else if (result.stuck) stuck++; else counts.push(result.orbs);
      done++;
    }
    // A setup that almost never finishes would take minutes to confirm.
    if (done >= 200 && (capped + stuck) / done > 0.5) { early = done < total; break; }
    $('sim-progress').textContent = t('sim.progress', num(done, 0), num(total, 0));
    await sleep(0);
  }
  simBusy = ''; $('sim-progress').textContent = '';
  simResult = { stats: stats(counts), bars: histogram(counts), done, capped, stuck, early, unit, orbName, signature };
  render();
  status(simStop ? t('sim.stopped', num(done, 0)) : t('sim.done', num(done, 0)), 'success');
}

async function watchSim() {
  if (simBusy || simReason()) return;
  const rule = rules[simOrb], effects = simEffects(), target = simTarget, before = item, ids = [simOrb, ...simOmens];
  const perSecond = Number($('sim-speed').value) || 50, frame = 1000 / Math.min(perSecond, 30), batch = Math.max(1, Math.round(perSecond / 30));
  let state = item, orbs = 0, end = '';
  simBusy = 'watch'; simStop = false; simPending = null; render();
  while (!end) {
    for (let i = 0; i < batch && !end; i++) {
      try { state = applyOrbOmens(state, data(), simOrb, rule, effects); orbs++; }
      catch { end = 'stuck'; break; }
      if (hasTarget(state, target)) end = 'hit';
      else if (orbs >= simCap) end = 'capped';
    }
    if (simStop && !end) end = 'stopped';
    item = state; renderItem();
    $('sim-progress').textContent = t('sim.watchCount', num(orbs, 0));
    if (!end) await sleep(frame);
  }
  // The result stays on the card (and the page locked) until it is kept or
  // reverted; only "keep" records it, with every orb and omen in the cost.
  simBusy = ''; simPending = { before, state, orbs, ids };
  $('sim-pending-text').textContent = t(`sim.watch.${end}`, num(orbs, 0));
  $('sim-keep').disabled = !orbs;
  render();
}

$('sim-orb').onchange = () => { simOrb = $('sim-orb').value; persist(); render(); };
$('sim-run').onclick = () => void runSim();
$('sim-watch').onclick = () => void watchSim();
$('sim-stop').onclick = () => { simStop = true; };
$('sim-keep').onclick = () => {
  if (!simPending) return;
  const { before, state, orbs, ids } = simPending; simPending = null; item = before; $('sim-progress').textContent = '';
  const usages = ids.map(id => ({ ...payment(id, rules[id] || omens[id]), quantity: orbs }));
  commit(state, t('sim.kept', num(orbs, 0), rules[simOrb].name), usages);
};
$('sim-revert').onclick = () => {
  if (!simPending) return;
  item = simPending.before; simPending = null; $('sim-progress').textContent = ''; render(); status(t('sim.reverted'));
};

// ---- Craft library -------------------------------------------------------
// Crafts saved by hand, and the craft a new one replaces (reset, class change,
// an item from the price check, opening a saved one) saved automatically.
// Nothing is written before the list has been read, so a slow start cannot
// overwrite the file with an empty list.
let library = [], libraryLoaded = false;
const craftState = () => ({ item, history, sessionStart });
const craftName = () => currentBase()?.name || itemLabel(item.base);
const libraryEntry = (name, auto) => entryFor(craftState(), { name, auto, baseName: craftName(), costEx: summarize(history).total || null });
function saveLibrary() {
  if (parent !== window) parent.postMessage({ type: 'craft-library-save', data: JSON.stringify(library) }, location.origin);
}
function autoSaveCurrent() {
  if (!libraryLoaded || !history.length || library.some(entry => sameCraft(entry, craftState()))) return;
  library = addEntry(library, libraryEntry(t('lib.autoName', craftName()), true));
  saveLibrary();
}
const shortTime = iso => new Date(iso).toLocaleString(locale(), { dateStyle: 'short', timeStyle: 'short' });
function renderLibrary() {
  const list = $('lib-list'); list.replaceChildren();
  $('lib-save').disabled = !libraryLoaded || Boolean(simBusy || simPending);
  if (!library.length) { list.append(element('li', libraryLoaded ? t('lib.empty') : t('loadingShort'), 'lib-empty')); return; }
  for (const entry of library) {
    const row = element('li', undefined, 'lib-entry');
    const head = element('div', undefined, 'lib-head');
    head.append(element('strong', entry.name || t('lib.unnamed')));
    if (entry.auto) head.append(element('em', t('lib.auto'), 'lib-tag'));
    if (entry.sanctified || entry.corrupted) head.append(element('em', entry.sanctified ? 'Sanctified' : 'Corrupted', 'lib-tag lock'));
    const meta = [entry.baseName || itemLabel(entry.page), entry.rarity, t('lib.mods', entry.mods), t('lib.steps', entry.steps)];
    if (entry.cost_ex) meta.push(`${exText(entry.cost_ex)} Ex`);
    meta.push(shortTime(entry.saved_at));
    const actions = element('div', undefined, 'lib-actions');
    const open = element('button', t('lib.open'));
    open.disabled = Boolean(simBusy || simPending);
    open.onclick = () => void openSaved(entry);
    // Deleting asks twice: the second click within three seconds deletes.
    const remove = element('button', t('lib.delete'));
    remove.onclick = () => {
      if (!remove.dataset.armed) {
        remove.dataset.armed = '1'; remove.textContent = t('lib.confirmDelete');
        setTimeout(() => { if (remove.isConnected) { delete remove.dataset.armed; remove.textContent = t('lib.delete'); } }, 3000);
        return;
      }
      library = removeEntry(library, entry.id); saveLibrary(); renderLibrary(); status(t('lib.deleted', entry.name));
    };
    actions.append(open, remove);
    row.append(head, element('small', meta.join(' · ')), actions);
    list.append(row);
  }
}
async function openSaved(entry) {
  if (simBusy || simPending || !ready) return;
  const saved = structuredClone(entry.state);
  if (!pages[saved.item.base]) { status(t('lib.badPage'), 'error'); return; }
  try { await load(saved.item.base); } catch (error) { status(error.message, 'error'); return; }
  autoSaveCurrent();
  undo.push({ item: clone(item), history: clone(history), sessionStart, activeOmens: clone(activeOmens) });
  item = { ...saved.item, baseSlots: currentBase(saved.item)?.slots || null };
  history = saved.history; sessionStart = saved.sessionStart || new Date().toISOString();
  render(); persist(); status(t('lib.opened', entry.name), 'success');
}
$('lib-save').onclick = () => {
  if (!libraryLoaded) return;
  const name = $('lib-name').value.trim() || `${craftName()} · ${shortTime(new Date().toISOString())}`;
  library = addEntry(library, libraryEntry(name, false));
  saveLibrary(); $('lib-name').value = ''; renderLibrary(); status(t('lib.saved', name), 'success');
};
$('lib-name').onkeydown = event => { if (event.key === 'Enter') $('lib-save').click(); };

$('download').onclick = () => {
  const url = URL.createObjectURL(new Blob([JSON.stringify({ format: 2, item, history, archives, sessionStart }, null, 2)], { type: 'application/json' }));
  const link = element('a'); link.href = url; link.download = 'theoretical-craft.json'; link.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000); status(t('saved'));
};

// The window opens with the app's language in the address; later changes
// arrive as messages.
setLang(new URLSearchParams(location.search).get('lang') || 'en');
document.documentElement.lang = 'en';
applyStatic();
// The status line is not a data-i18n text: a language switch must not wipe
// the last message.
status(t('loading'));

try {
  const [loadedRules, loadedManifest, specialData, classData, loadedBases] = await Promise.all([
    readJSON('data/currency-rules.source.json'), readJSON('data/manifest.json'),
    readJSON('data/special-currencies.json'), readJSON('data/classes.json'), readJSON('data/bases.json'),
  ]);
  baseData = loadedBases;
  rules = { ...loadedRules, ...catalystRules(), ...infuserRules() }; manifest = loadedManifest; classes = classData.classes; bases = classData.bases || {};
  for (const cls of classes) for (const v of cls.variants) pages[v.page] = { cls, attr: v.attr };
  if (rules['greater-orb-of-augmentation']) rules['greater-orb-of-augmentation'].name = 'Greater Orb of Augmentation';
  // PoE2DB names the Vaal Orb after the Perfect Exalted Orb.
  if (rules['vaal-orb']) rules['vaal-orb'].name = 'Vaal Orb';
  specials = specialData.rules; runes = specialData.runes || {};
  omens = omenDefinitions(rules);
  if (omens['omen-of-the-liege']) omens['omen-of-the-liege'].icon = 'Art/2DItems/Currency/Omens/OmenOnAbyssGuarenteedLichTypeMod2.webp';
  let restored = false;
  try {
    const saved = JSON.parse(localStorage.getItem(storageKey));
    if (saved?.format === 2 && pages[saved.item?.base] && Array.isArray(saved.history) && Array.isArray(saved.item.mods)) {
      item = saved.item; history = saved.history; archives = saved.archives || []; sessionStart = saved.sessionStart;
      priceOverrides = saved.priceOverrides || {}; restored = true;
      mode = ['basic','desecrate','essence'].includes(saved.mode) ? saved.mode : 'basic';
      if (saved.selected && supported(saved.selected) && rules[saved.selected]) { selected = saved.selected; tier = rules[selected].tier || ''; }
      for (const context of ['desecrate','essence']) if (specials[saved.specialRemember?.[context]]?.operation === context) specialRemember[context] = saved.specialRemember[context];
      if (saved.activeOmens && ['basic','desecrate','essence'].every(k=>Array.isArray(saved.activeOmens[k]))) activeOmens = saved.activeOmens;
      keepOmens = saved.keepOmens === true;
      if (mode !== 'basic') specialSelected = specialRemember[mode];
      pool = typeof saved.pool === 'string' ? saved.pool : mode === 'desecrate' ? 'desecrated' : mode === 'essence' ? 'essence' : 'normal';
      $('pool').value = pool;
      if (saved.sim) {
        if (saved.sim.target?.key && Number.isFinite(saved.sim.target.tier)) simTarget = saved.sim.target;
        if (typeof saved.sim.orb === 'string' && rules[saved.sim.orb]) simOrb = saved.sim.orb;
        if (Array.isArray(saved.sim.omens)) simOmens = saved.sim.omens.filter(id => omens[id]);
      }
    }
  } catch { /* A corrupt stored draft cannot prevent opening the lab. */ }
  if (!pages[item.base]) item = createItem(classes[0].variants[0].page);
  item = { ...item, baseSlots: currentBase(item)?.slots || null };
  await load(item.base);
  applyStatic();
  ready = true;
  for (const id of ['item-class', 'variant', 'ilvl', 'quality', 'rarity', 'search', 'pool', 'download', 'reset']) $(id).disabled = false;
  $('workbench').setAttribute('aria-busy', 'false'); render();
  status(restored ? t('restored') : t('ready'));
} catch (error) {
  $('workbench').setAttribute('aria-busy', 'false'); status(t('load.failed', error.message), 'error');
  $('item-mods').replaceChildren(element('p', t('load.failedShort'), 'empty-item'));
}

const parentOrigin = location.origin;
$('price-check').onclick = () => {
  try {
    $('price-check').disabled = true;
    parent.postMessage({type:'craft-price',raw:craftText({ ...item, quality: qualityOf(item), sockets: socketsOf(item) }, classOf(item.base)?.itemClass, currentBase())},parentOrigin);
  } catch (error) { status(error.message,'error'); renderItem(); }
};
window.addEventListener('message',event => {
  if (event.source !== parent || event.origin !== parentOrigin) return;
  if (event.data?.type === 'craft-theme') {
    applyThemePalette(event.data);
  } else if (event.data?.type === 'craft-prices' && Array.isArray(event.data.prices?.currency)) {
    prices = {...event.data.prices,origin:'app'};
    render();
  } else if (event.data?.type === 'craft-icons' && Array.isArray(event.data.currencies)) {
    icons = iconIndex(event.data.currencies);
    if (ready) render();
  } else if (event.data?.type === 'craft-import' && event.data.item && typeof event.data.item.raw === 'string') {
    if (ready) void importCopied(event.data.item);
  } else if (event.data?.type === 'craft-lang' && typeof event.data.lang === 'string') {
    setLang(event.data.lang); applyStatic(); render();
  } else if (event.data?.type === 'craft-library') {
    if (event.data.error) status(t('lib.loadFailed', event.data.error), 'error');
    else { library = parseLibrary(event.data.data); libraryLoaded = true; renderLibrary(); }
  } else if (event.data?.type === 'craft-library-saved' && event.data.error) {
    status(t('lib.saveFailed', event.data.error), 'error');
  } else if (event.data?.type === 'craft-result') {
    status(event.data.error || event.data.message || t('market.sent'),event.data.error ? 'error' : 'success');
    renderItem();
  }
});
parent.postMessage({type:'craft-ready'},parentOrigin);
parent.postMessage({type:'craft-library-load'},parentOrigin);
