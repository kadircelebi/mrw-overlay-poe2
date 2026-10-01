import { createItem, count, limits, manualAdd, manualReason, removeMod, clearMods,
  setRarity, candidates, currencyReason, applyCurrency, supported, rolledText, replaceTier, sortedMods } from './engine.mjs';
import { applicable, essenceRows, specialReason, applySpecial, revealChoice } from './special.mjs';
import { usageEntry, summarize } from './ledger.mjs';
import { craftText } from './trade.mjs';
import { iconIndex, iconFor } from './icons.mjs';
import { omenDefinitions, relevantOmens, omenEffects, filterOmenRows, orbOmenReason, applyOrbOmens } from './omens.mjs';
import { t, setLang, locale, num, variantName } from './i18n.mjs';
import { pageFor, importItem } from './import.mjs';

const $ = id => document.getElementById(id);
const clone = object => structuredClone(object);
const shortNames = { transmute: 'Transmutation', aug: 'Augmentation', regal: 'Regal', exalted: 'Exalted', chaos: 'Chaos', annu: 'Annulment', divine: 'Divine' };
const order = ['transmute', 'aug', 'regal', 'exalted', 'chaos', 'annu', 'divine'];
let item = createItem(), datasets = {}, rules = {}, selected = 'transmute', tier = '', pool = 'normal';
let history = [], undo = [], ready = false;
let specials = {}, specialSelected = 'preserved-rib', prices = {currency:[]}, priceOverrides = {}, archives = [], sessionStart = new Date().toISOString();
// Legacy archives remain in saved/exported data, but are no longer created or displayed.
const storageKey = 'mrw-craft-v1';
let mode = 'basic', omens = {}, activeOmens = {basic:[],desecrate:[],essence:[]};
const specialRemember = {desecrate:'preserved-rib',essence:'greater-essence-of-the-mind'};
// Item classes and their defence variants; item.base is the data page
// ("Gloves_str", "Rings"), so drafts saved before other classes still load.
let classes = [], pages = {}, bases = {}, manifest = null;
const snapshot = state => ({base:state.base,rarity:state.rarity,ilvl:state.ilvl,
  mods:state.mods.map(m => ({source_id:m.source_id,pool:m.pool,affix:m.affix,tier:m.tier,values:m.values,desecrated:Boolean(m.desecrated)}))});
function persist() {
  try { localStorage.setItem(storageKey,JSON.stringify({format:2,item,history,archives,priceOverrides,sessionStart,mode,activeOmens,selected,tier,specialSelected,specialRemember,pool})); }
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
  if (newSession && !history.length && JSON.stringify(snapshot(item)) === JSON.stringify(snapshot(next))) return;
  undo.push({ item: clone(item), history: clone(history), sessionStart,activeOmens:clone(activeOmens) });
  if (undo.length > 100) undo.shift();
  const before = snapshot(item);
  if (newSession) {
    history = []; sessionStart = new Date().toISOString();
  }
  item = next;
  const usages = Array.isArray(usage) ? usage : usage ? [usage] : [];
  if (!newSession) history.unshift({ time: new Date().toISOString(), label, usages, before, after:snapshot(next) });
  for (const context of Object.keys(activeOmens)) activeOmens[context] = activeOmens[context].filter(id => !usages.some(u => u.id === id));
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
const visibleRules = () => Object.entries(rules).filter(([id, r]) => supported(id) && (r.tier || '') === tier)
  .sort(([a], [b]) => order.indexOf(kind(a)) - order.indexOf(kind(b)));
function kind(id) { return id.includes('transmutation') ? 'transmute' : id.includes('augmentation') ? 'aug' : order.find(k => id.includes(k)) || id; }

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
  $('source-link').href = `https://poe2db.tw/us/${encodeURIComponent(item.base)}#ModifiersCalc`;
}

function renderItem() {
  renderClassPicker();
  $('rarity').value = item.rarity;
  $('ilvl').value = item.ilvl;
  for (const id of ['rarity','item-class','variant','ilvl','clear','reset']) $(id).disabled = Boolean(item.reveal);
  $('item-card').className = 'item-card ' + item.rarity;
  $('item-name').textContent = itemLabel(item.base);
  $('rarity-label').textContent = item.rarity;
  $('item-level').textContent = item.ilvl;
  $('prefix-count').textContent = `${count(item, 'Prefix')} / ${limits[item.rarity]}`;
  $('suffix-count').textContent = `${count(item, 'Suffix')} / ${limits[item.rarity]}`;
  const mods = $('item-mods'); mods.replaceChildren();
  if (!item.mods.length) mods.append(element('p', item.rarity === 'Rare' ? t('empty.rare') : t('empty.other'), 'empty-item'));
  sortedMods(item).forEach(({ mod, index }) => {
    const row = element('div', undefined, 'item-mod');
    const copy = element('div');
    copy.append(element('small', `${mod.affix === 'Prefix' ? 'P' : 'S'}${mod.tier} · ${mod.name} · ${mod.desecrated ? 'Desecrated' : mod.pool}`), element('p', rolledText(mod), 'mod-value'));
    const remove = element('button', '×', 'remove');
    remove.title = t('mod.remove'); remove.setAttribute('aria-label', t('mod.removeAria', mod.name));
    remove.disabled = Boolean(item.reveal);
    remove.onclick = () => commit(removeMod(item, index), t('mod.removed', mod.name, item.rarity));
    row.append(copy, remove);
    if (mod.ranges.length) {
      const editor = element('div', undefined, 'roll-editor');
      editor.append(element('span', 'Roll'));
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
      row.append(editor);
    }
    mods.append(row);
  });
  $('clear').disabled = !item.mods.length || Boolean(item.reveal);
  $('undo').disabled = !undo.length;
  $('reset').disabled = Boolean(item.reveal) || (item.rarity === 'Normal' && !item.mods.length && !history.length);
  const visibleHistory = history;
  const cost = summarize(visibleHistory);
  $('cost-total').textContent = `${num(cost.total)} Exalted${cost.unknown ? t('cost.unknown', cost.unknown) : ''}`;
  $('price-info').textContent = prices.generated_at ? t('price.info', prices.league, new Date(prices.generated_at).toLocaleString(locale())) : t('price.waiting');
  $('price-check').disabled = Boolean(item.reveal);
  $('cost-breakdown').replaceChildren();
  for (const row of cost.rows) $('cost-breakdown').append(element('li',`${row.quantity} × ${row.name} · ${num(row.cost_ex)} Ex${row.unknown ? t('cost.missing') : ''}`));
  $('history').replaceChildren();
  for (const entry of visibleHistory) {
    const li = element('li'); const time = element('time', new Date(entry.time).toLocaleTimeString(locale(), { hour: '2-digit', minute: '2-digit' }));
    time.dateTime = entry.time; li.append(time, document.createTextNode(entry.label));
    for (const usage of entry.usages || (entry.usage ? [entry.usage] : [])) li.append(element('small',` · ${usage.quantity} × ${usage.name}: ${usage.unit_ex === null ? t('price.none') : num(usage.unit_ex)+' Ex'}`));
    $('history').append(li);
  }
  if (!visibleHistory.length) $('history').append(element('li', t('history.empty')));
}

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
  activeOmens[context] = activeOmens[context].filter(id => options.some(([key]) => key === id));
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
}
const effectsFor = (context,id,rule) => omenEffects(omens,activeOmens[context],id,rule,data());
const operationPayments = (context,id,rule) => [payment(id,rule),...activeOmens[context].map(id => payment(id,omens[id]))];
function renderSpecials() {
  const context = mode === 'essence' ? 'essence' : 'desecrate';
  const visible = Object.entries(specials).filter(([,r]) => applicable(r,data()) &&
    r.operation === context && (r.operation === 'desecrate' || essenceRows(data(),r).length));
  if (!visible.some(([id]) => id === specialSelected)) specialSelected = visible[0]?.[0] || '';
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
  $('special-price').value = usage.unit_ex ?? '';
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
  const choices = $('reveal-choices'); choices.replaceChildren();
  if (item.reveal) {
    choices.append(element('h3',t('reveal.title')));
    item.reveal.choices.forEach((m,i) => {
      const button = element('button',`${m.affix === 'Prefix' ? 'P' : 'S'}${m.tier} · ${rolledText(m)}`);
      button.setAttribute('aria-label',t('reveal.option', i+1));
      button.onclick = () => commit(revealChoice(item,i),t('reveal.picked', m.name));
      choices.append(button);
    });
  }
}

function renderCurrencies() {
  $('currencies').replaceChildren();
  for (const [id, rule] of visibleRules()) {
    const reason = currencyReason(item, data(), id, rule);
    const button = element('button', undefined, `currency${reason ? ' unavailable' : ''}`);
    button.dataset.currency = id; button.setAttribute('aria-pressed', String(mode === 'basic' && selected === id));
    button.setAttribute('aria-label', rule.name + (rule.tier ? ` ${rule.tier}` : ''));
    button.title = reason ? `${rule.name}: ${reason}` : rule.name;
    const img = iconElement(rule.icon, true);
    button.append(img, element('span', shortNames[kind(id)] || rule.name));
    button.onclick = () => { selected = id; selectMode('basic'); status(reason || t('currency.applyHint')); };
    $('currencies').append(button);
  }
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
    button.onclick = () => selectMode('desecrate',id);
    $('currencies').append(button);
  }
  // Not every class has an essence (or the remembered one); show the button
  // with the first essence that fits this class.
  const essences = Object.entries(specials).filter(([,r]) => r.operation === 'essence' && applicable(r,data()) && essenceRows(data(),r).length);
  if (essences.length) {
    const essenceRule = specials[specialRemember.essence] && essences.some(([id]) => id === specialRemember.essence)
      ? specials[specialRemember.essence] : essences[0][1];
    const essenceButton = element('button',undefined,'currency');
    essenceButton.setAttribute('aria-label','Essence');
    essenceButton.setAttribute('aria-pressed',String(mode === 'essence'));
    essenceButton.title = t('essence.pick');
    essenceButton.append(iconElement(essenceRule.icon, true),element('span','Essence'));
    essenceButton.onclick = () => selectMode('essence');
    $('currencies').append(essenceButton);
  }
  const rule = rules[selected];
  renderOmens('basic',selected,rule);
  const omenEffect = effectsFor('basic',selected,rule);
  const reason = orbOmenReason(item,data(),selected,rule,omenEffect);
  $('selected-name').textContent = rule.name + (rule.tier ? ` · ${rule.tier}` : '');
  $('selected-detail').textContent = reason || `${t(`effect.${rule.afterTrigger}`)}${rule.afterRarity && rule.afterRarity !== item.rarity ? t('effect.becomes', rule.afterRarity) : ''}${rule.beforeMin_mod_lv ? t('effect.minLevel', rule.beforeMin_mod_lv) : ''}`;
  $('apply').disabled = Boolean(reason);
  if (!reason && activeOmens.basic.length) $('selected-detail').textContent =
    t('effect.omens', omenEffect.quantity, omenEffect.side ? t('effect.side', omenEffect.side) : '');
}

function renderMods() {
  const open = new Set([...document.querySelectorAll('details[open]')].map(n => n.dataset.family));
  const search = $('search').value.trim().toLowerCase();
  let listEffects = {side:null,tags:[],quantity:1};
  if (mode === 'desecrate' && specials[specialSelected]) listEffects = effectsFor('desecrate',specialSelected,specials[specialSelected]);
  if (mode === 'desecrate' && item.reveal?.effects) listEffects = item.reveal.effects;
  if (mode === 'basic') listEffects = effectsFor('basic',selected,rules[selected]);
  const rows = filterOmenRows(data().mods.filter(m => m.pool === pool && ['Prefix', 'Suffix'].includes(m.affix) &&
    (!['essence','perfect_essence'].includes(pool) || Object.values(specials).some(r => r.operation === 'essence' &&
      essenceRows(data(),r).some(row => row.source_id === m.source_id && row.pool === m.pool)))),listEffects);
  const potential = filterOmenRows(candidates(item, data(), { pool, rarity: 'Rare' }),listEffects);
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
    $(side.toLowerCase() + '-total').textContent = t('weight', num(total));
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
      summary.append(element('span', title, 'family-name'), element('span', existing ?
        t('family.onItem', visibleTier ? `T${visibleTier}` : existing.name) : t('family.tiers', mods.length), 'family-count'));
      if (existing) details.classList.add('has-selected');
      const familyList = element('div', undefined, 'family-list');
      for (const row of mods.sort((a, b) => a.tier - b.tier)) {
        const active = existing && (existing.source_id === row.source_id || existing.text === row.text);
        const entry = element('div', undefined, `tier-row${active ? ' selected-tier' : ''}`);
        const description = element('div', undefined, 'tier-description');
        description.append(element('p', row.text));
        const eligible = potential.some(m => m.source_id === row.source_id);
        const share = eligible && total ? `${num(row.weight / total * 100, 3)}%` : '—';
        description.append(element('small', `${row.name} · ilvl ${row.required_ilvl} · w ${row.weight} · ${share}`));
        const button = element('button', active ? '✓' : existing ? '↔' : '+');
        const reason = manualReason(existing ? removeMod(item, existingIndex) : item, row);
        button.disabled = active || Boolean(reason);
        button.title = active ? t('tier.onItem') : reason || (existing ? t('tier.replace') : t('tier.add'));
        button.setAttribute('aria-label', t('tier.aria', row.name, row.tier, t(active ? 'tier.aria.on' : existing ? 'tier.aria.replace' : 'tier.aria.add')));
        button.onclick = () => attempt(() => commit(existing ? replaceTier(item, existingIndex, row) : manualAdd(item, row),
          existing ? t('tier.replaced', existing.name, existing.tier, row.name, row.tier) : t('tier.added', row.name, row.tier)));
        entry.append(element('span', `T${row.tier}`, 'tier-id'), description, button); familyList.append(entry);
      }
      details.append(summary, familyList); list.append(details);
    }
    if (!list.children.length) list.append(element('p', t('pool.empty'), 'empty-pool'));
  }
}

function render() { if (ready) {
  $('basic-controls').hidden = mode !== 'basic'; $('special-controls').hidden = mode === 'basic';
  document.querySelectorAll('[data-tier]').forEach(b => b.setAttribute('aria-pressed',String(b.dataset.tier === tier)));
  renderItem(); renderSpecials(); renderCurrencies(); renderMods();
} }

// Switching class or defence type starts a new item, like the old base picker.
async function switchPage(page) {
  if (!pages[page] || page === item.base) return;
  try {
    await load(page);
    commit({ ...createItem(page), ilvl: item.ilvl }, t('class.changed'), null, true);
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
$('reset').onclick = () => commit({ ...createItem(item.base), ilvl: item.ilvl }, t('reset.done'),null,true);
$('undo').onclick = () => {
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
$('apply').onclick = () => attempt(() => {
  const rule = rules[selected];
  const effects = effectsFor('basic',selected,rule), payments = operationPayments('basic',selected,rule);
  commit(applyOrbOmens(item,data(),selected,rule,effects), t('applied', payments.map(p=>p.name).join(' + ')),payments);
});
document.querySelectorAll('[data-tier]').forEach(button => button.onclick = () => {
  if (!ready) return;
  tier = button.dataset.tier;
  document.querySelectorAll('[data-tier]').forEach(b => b.setAttribute('aria-pressed', String(b === button)));
  selected = visibleRules().find(([id]) => kind(id) === kind(selected))?.[0] || visibleRules()[0][0];
  render(); status(t('tier.changed'));
});
// The price check's ⚒ button sends the item the player copied in game.
async function importCopied(copied) {
  const where = pageFor(copied, { classes, bases });
  if (where.error) { status(where.error === 'unique' ? t('import.unique') : t('import.class', copied.class || '?'), 'error'); return; }
  try {
    const { item: next, unmatched } = importItem(copied, where.page, await load(where.page));
    mode = 'basic'; pool = 'normal'; $('pool').value = pool;
    commit(next, t('import.done', itemLabel(where.page), next.mods.length) +
      (unmatched.length ? t('import.unmatched', unmatched.length, unmatched.join(', ')) : '') +
      (where.guessed ? t('import.guessed') : ''), null, true);
    if (unmatched.length || where.guessed) status($('status').textContent, 'error');
  } catch (error) { status(error.message, 'error'); }
}

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
  const [loadedRules, loadedManifest, specialData, classData] = await Promise.all([
    readJSON('data/currency-rules.source.json'), readJSON('data/manifest.json'),
    readJSON('data/special-currencies.json'), readJSON('data/classes.json'),
  ]);
  rules = loadedRules; manifest = loadedManifest; classes = classData.classes; bases = classData.bases || {};
  for (const cls of classes) for (const v of cls.variants) pages[v.page] = { cls, attr: v.attr };
  if (rules['greater-orb-of-augmentation']) rules['greater-orb-of-augmentation'].name = 'Greater Orb of Augmentation';
  specials = specialData.rules;
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
      if (mode !== 'basic') specialSelected = specialRemember[mode];
      pool = ['normal','marksman','decay','desecrated','essence','perfect_essence'].includes(saved.pool) ? saved.pool : mode === 'desecrate' ? 'desecrated' : mode === 'essence' ? 'essence' : 'normal';
      $('pool').value = pool;
    }
  } catch { /* A corrupt stored draft cannot prevent opening the lab. */ }
  if (!pages[item.base]) item = createItem(classes[0].variants[0].page);
  await load(item.base);
  applyStatic();
  ready = true;
  for (const id of ['item-class', 'variant', 'ilvl', 'rarity', 'search', 'pool', 'download', 'reset']) $(id).disabled = false;
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
    parent.postMessage({type:'craft-price',raw:craftText(item, classOf(item.base)?.itemClass)},parentOrigin);
  } catch (error) { status(error.message,'error'); renderItem(); }
};
window.addEventListener('message',event => {
  if (event.source !== parent || event.origin !== parentOrigin) return;
  if (event.data?.type === 'craft-prices' && Array.isArray(event.data.prices?.currency)) {
    prices = {...event.data.prices,origin:'app'};
    render();
  } else if (event.data?.type === 'craft-icons' && Array.isArray(event.data.currencies)) {
    icons = iconIndex(event.data.currencies);
    if (ready) render();
  } else if (event.data?.type === 'craft-import' && event.data.item && typeof event.data.item.raw === 'string') {
    if (ready) void importCopied(event.data.item);
  } else if (event.data?.type === 'craft-lang' && typeof event.data.lang === 'string') {
    setLang(event.data.lang); applyStatic(); render();
  } else if (event.data?.type === 'craft-result') {
    status(event.data.error || t('market.sent'),event.data.error ? 'error' : 'success');
    renderItem();
  }
});
parent.postMessage({type:'craft-ready'},parentOrigin);
