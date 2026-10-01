import { createItem, count, limits, manualAdd, manualReason, removeMod, clearMods,
  setRarity, candidates, currencyReason, applyCurrency, supported, rolledText, replaceTier, sortedMods } from './engine.mjs';
import { applicable, essenceRows, specialReason, applySpecial, revealChoice } from './special.mjs';
import { usageEntry, summarize } from './ledger.mjs';
import { craftText } from './trade.mjs';
import { omenDefinitions, relevantOmens, omenEffects, filterOmenRows, orbOmenReason, applyOrbOmens } from './omens.mjs';

const $ = id => document.getElementById(id);
const clone = object => structuredClone(object);
const labels = { Gloves_str: 'Zırh Eldivenleri', Gloves_dex: 'Evasion Eldivenleri', Gloves_int: 'Energy Shield Eldivenleri',
  Gloves_str_dex: 'Zırh / Evasion Eldivenleri', Gloves_str_int: 'Zırh / Energy Shield Eldivenleri', Gloves_dex_int: 'Evasion / Energy Shield Eldivenleri' };
const shortNames = { transmute: 'Transmutation', aug: 'Augmentation', regal: 'Regal', exalted: 'Exalted', chaos: 'Chaos', annu: 'Annulment', divine: 'Divine' };
const order = ['transmute', 'aug', 'regal', 'exalted', 'chaos', 'annu', 'divine'];
let item = createItem(), datasets = {}, rules = {}, selected = 'transmute', tier = '', pool = 'normal';
let history = [], undo = [], ready = false;
let specials = {}, specialSelected = 'preserved-rib', prices = {currency:[]}, priceOverrides = {}, archives = [], sessionStart = new Date().toISOString();
// Legacy archives remain in saved/exported data, but are no longer created or displayed.
const storageKey = 'mrw-craft-v1';
let mode = 'basic', omens = {}, activeOmens = {basic:[],desecrate:[],essence:[]};
const specialRemember = {desecrate:'preserved-rib',essence:'greater-essence-of-the-mind'};
const snapshot = state => ({base:state.base,rarity:state.rarity,ilvl:state.ilvl,
  mods:state.mods.map(m => ({source_id:m.source_id,pool:m.pool,affix:m.affix,tier:m.tier,values:m.values,desecrated:Boolean(m.desecrated)}))});
function persist() {
  try { localStorage.setItem(storageKey,JSON.stringify({format:2,item,history,archives,priceOverrides,sessionStart,mode,activeOmens,selected,tier,specialSelected,specialRemember,pool})); }
  catch { status('Geçmiş tarayıcıya kaydedilemedi. Kaydet düğmesiyle dosya alabilirsin.','error'); }
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
const visibleRules = () => Object.entries(rules).filter(([id, r]) => supported(id) && (r.tier || '') === tier)
  .sort(([a], [b]) => order.indexOf(kind(a)) - order.indexOf(kind(b)));
function kind(id) { return id.includes('transmutation') ? 'transmute' : id.includes('augmentation') ? 'aug' : order.find(k => id.includes(k)) || id; }

function renderItem() {
  $('rarity').value = item.rarity;
  $('base').value = item.base;
  $('ilvl').value = item.ilvl;
  for (const id of ['rarity','base','ilvl','clear','reset']) $(id).disabled = Boolean(item.reveal);
  $('item-card').className = 'item-card ' + item.rarity;
  $('item-name').textContent = labels[item.base];
  $('rarity-label').textContent = item.rarity;
  $('item-level').textContent = item.ilvl;
  $('prefix-count').textContent = `${count(item, 'Prefix')} / ${limits[item.rarity]}`;
  $('suffix-count').textContent = `${count(item, 'Suffix')} / ${limits[item.rarity]}`;
  const mods = $('item-mods'); mods.replaceChildren();
  if (!item.mods.length) mods.append(element('p', item.rarity === 'Rare'
    ? 'Rare eşya hazır. Exalted ile başlayabilir veya listeden affix ekleyebilirsin.'
    : 'Henüz affix yok. Currency kullan veya sağdaki listeden bir mod seç.', 'empty-item'));
  sortedMods(item).forEach(({ mod, index }) => {
    const row = element('div', undefined, 'item-mod');
    const copy = element('div');
    copy.append(element('small', `${mod.affix === 'Prefix' ? 'P' : 'S'}${mod.tier} · ${mod.name} · ${mod.desecrated ? 'Desecrated' : mod.pool}`), element('p', rolledText(mod), 'mod-value'));
    const remove = element('button', '×', 'remove');
    remove.title = 'Affix’i kaldır'; remove.setAttribute('aria-label', `${mod.name} affix’ini kaldır`);
    remove.disabled = Boolean(item.reveal);
    remove.onclick = () => commit(removeMod(item, index), `${mod.name} kaldırıldı; ${item.rarity} korundu.`);
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
            input.setAttribute('aria-invalid', 'true'); throw new Error(`Roll ${range.min}–${range.max} aralığında olmalı.`);
          }
          const next = clone(item); next.mods[index].values[rangeIndex] = value;
          commit(next, `${mod.name} roll değeri ${value} yapıldı.`);
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
  $('cost-total').textContent = `${cost.total.toLocaleString('tr-TR',{maximumFractionDigits:6})} Exalted${cost.unknown ? ` + ${cost.unknown} fiyatı bilinmeyen kullanım` : ''}`;
  $('price-info').textContent = prices.generated_at ? `${prices.league} · fiyat: ${new Date(prices.generated_at).toLocaleString('tr-TR',{timeZone:'Europe/Istanbul'})} · Mevcut craft` : 'Uygulama fiyat listesi bekleniyor; bilinmeyen maliyetler ayrı gösterilir.';
  $('price-check').disabled = Boolean(item.reveal);
  $('cost-breakdown').replaceChildren();
  for (const row of cost.rows) $('cost-breakdown').append(element('li',`${row.quantity} × ${row.name} · ${row.cost_ex.toLocaleString('tr-TR',{maximumFractionDigits:6})} Ex${row.unknown ? ' + fiyat eksik' : ''}`));
  $('history').replaceChildren();
  for (const entry of visibleHistory) {
    const li = element('li'); const time = element('time', new Date(entry.time).toLocaleTimeString('tr-TR', { hour: '2-digit', minute: '2-digit', timeZone: 'Europe/Istanbul' }));
    time.dateTime = entry.time; li.append(time, document.createTextNode(entry.label));
    for (const usage of entry.usages || (entry.usage ? [entry.usage] : [])) li.append(element('small',` · ${usage.quantity} × ${usage.name}: ${usage.unit_ex === null ? 'fiyat bilinmiyor' : usage.unit_ex.toLocaleString('tr-TR',{maximumFractionDigits:6})+' Ex'}`));
    $('history').append(li);
  }
  if (!visibleHistory.length) $('history').append(element('li', 'Henüz işlem yok.'));
}

function payment(id,rule) {
  const usage = usageEntry(id,rule.name,prices);
  if (Object.hasOwn(priceOverrides,id)) { usage.unit_ex = priceOverrides[id]; usage.source = 'Kullanıcının birim fiyatı'; }
  return usage;
}
function renderOmens(context,id,rule) {
  const list = $(context === 'basic' ? 'orb-omens' : 'special-omens'); list.replaceChildren();
  const options = relevantOmens(omens,id,rule,data());
  activeOmens[context] = activeOmens[context].filter(id => options.some(([key]) => key === id));
  if (!options.length) return;
  list.append(element('strong','Bir sonraki kullanım için Omen'));
  for (const [key,omen] of options) {
    const label = element('label'); const input = element('input'); input.type = 'checkbox';
    input.checked = activeOmens[context].includes(key); input.disabled = Boolean(item.reveal);
    input.setAttribute('aria-label',omen.name);
    const img = element('img'); img.alt = ''; img.src = `assets/currency/${omen.icon.split('/').at(-1)}`;
    img.onerror = () => { img.hidden = true; };
    const usage = payment(key,omen);
    label.append(input,img,element('span',omen.name),element('small',usage.unit_ex === null ? 'Fiyat yok' : `${usage.unit_ex.toLocaleString('tr-TR',{maximumFractionDigits:4})} Ex`));
    input.onchange = () => {
      if (input.checked) activeOmens[context] = [...activeOmens[context].filter(id => !omen.exclusives?.includes(id)),key];
      else activeOmens[context] = activeOmens[context].filter(id => id !== key);
      render(); persist(); status('Omen seçimi güncellendi; başarılı kullanımda bir adet tüketilir.');
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
    const group = element('optgroup'); group.label = op === 'desecrate' ? 'Desecrate · uygun kemik' : 'Essence · uygun garantili mod';
    for (const [id,r] of visible.filter(([,r]) => r.operation === op)) group.append(new Option(r.name,id));
    $('special').append(group);
  }
  $('special').value = specialSelected;
  const rule = specials[specialSelected]; if (!rule) return;
  renderOmens(context,specialSelected,rule);
  $('special').disabled = Boolean(item.reveal);
  const usage = payment(specialSelected,rule);
  $('special-price').disabled = Boolean(item.reveal);
  $('special-price').value = usage.unit_ex ?? '';
  $('special-price').placeholder = 'Fiyat bilinmiyor';
  $('special-name').textContent = rule.name;
  $('special-icon').src = `assets/currency/${rule.icon.split('/').at(-1)}`;
  $('special-icon').hidden = !rule.icon;
  $('special-icon').onerror = () => { $('special-icon').hidden = true; };
  const effects = effectsFor(context,specialSelected,rule);
  const reason = specialReason(item,data(),rule,effects);
  $('special-detail').textContent = reason || (rule.operation === 'desecrate'
    ? `Unrevealed affix ekler; ardından en fazla 3 seçenekten birini seçersin.${item.mods.length === 6 ? ' Dolu eşyadan rastgele bir affix kaldırılır.' : ''}`
    : `${rule.removes ? 'Rastgele bir affix kaldırır; garantili mod ekler.' : 'Magic eşyaya garantili mod ekler; Rare olur.'} ${essenceRows(data(),rule).map(m => m.text).join(' / ')}`);
  $('special-apply').disabled = Boolean(reason);
  $('desecrate-note').hidden = context !== 'desecrate';
  const choices = $('reveal-choices'); choices.replaceChildren();
  if (item.reveal) {
    choices.append(element('h3','Well of Souls · bir affix seç'));
    item.reveal.choices.forEach((m,i) => {
      const button = element('button',`${m.affix === 'Prefix' ? 'P' : 'S'}${m.tier} · ${rolledText(m)}`);
      button.setAttribute('aria-label',`Desecrate seçenek ${i+1}`);
      button.onclick = () => commit(revealChoice(item,i),`${m.name} Desecrate açılımından seçildi.`);
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
    const img = element('img'); img.alt = ''; img.src = `assets/currency/${rule.icon.split('/').at(-1)}`;
    img.onerror = () => img.replaceWith(element('span', '◇', 'orb-fallback'));
    button.append(img, element('span', shortNames[kind(id)] || rule.name));
    button.onclick = () => { selected = id; selectMode('basic'); status(reason || 'Uygula ile bir sanal craft adımı yap.'); };
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
    const img = element('img'); img.alt = ''; img.src = `assets/currency/${rule.icon.split('/').at(-1)}`;
    button.append(img,element('span',rule.name));
    button.onclick = () => selectMode('desecrate',id);
    $('currencies').append(button);
  }
  const essenceRule = specials[specialRemember.essence];
  const essenceButton = element('button',undefined,'currency');
  essenceButton.setAttribute('aria-label','Essence');
  essenceButton.setAttribute('aria-pressed',String(mode === 'essence'));
  essenceButton.title = 'Essence seç';
  const essenceImg = element('img'); essenceImg.alt = '';
  essenceImg.src = `assets/currency/${essenceRule.icon.split('/').at(-1)}`;
  essenceButton.append(essenceImg,element('span','Essence'));
  essenceButton.onclick = () => selectMode('essence');
  $('currencies').append(essenceButton);
  const rule = rules[selected];
  renderOmens('basic',selected,rule);
  const omenEffect = effectsFor('basic',selected,rule);
  const reason = orbOmenReason(item,data(),selected,rule,omenEffect);
  $('selected-name').textContent = rule.name + (rule.tier ? ` · ${rule.tier}` : '');
  const effects = { add: 'Ağırlıklara göre 1 affix ekler.', del: 'Rastgele 1 affix kaldırır; rarity korunur.',
    del_add: 'Rastgele 1 affix kaldırıp ağırlıklara göre yenisini ekler.', divine: 'Affix tier’larını koruyup roll değerlerini değiştirir.' };
  $('selected-detail').textContent = reason || `${effects[rule.afterTrigger]}${rule.afterRarity && rule.afterRarity !== item.rarity ? ` Eşya ${rule.afterRarity} olur.` : ''}${rule.beforeMin_mod_lv ? ` Minimum mod seviyesi: ${rule.beforeMin_mod_lv}.` : ''}`;
  $('apply').disabled = Boolean(reason);
  if (!reason && activeOmens.basic.length) $('selected-detail').textContent =
    `${omenEffect.quantity} affix ekler${omenEffect.side ? ` · yalnız ${omenEffect.side}` : ''}. Seçili omen’lar tüketilir.`;
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
    $(side.toLowerCase() + '-total').textContent = `Ağırlık ${total.toLocaleString('tr-TR')}`;
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
        `${visibleTier ? `T${visibleTier}` : existing.name} eşyada` : `${mods.length} tier`, 'family-count'));
      if (existing) details.classList.add('has-selected');
      const familyList = element('div', undefined, 'family-list');
      for (const row of mods.sort((a, b) => a.tier - b.tier)) {
        const active = existing && (existing.source_id === row.source_id || existing.text === row.text);
        const entry = element('div', undefined, `tier-row${active ? ' selected-tier' : ''}`);
        const description = element('div', undefined, 'tier-description');
        description.append(element('p', row.text));
        const eligible = potential.some(m => m.source_id === row.source_id);
        const share = eligible && total ? `${(row.weight / total * 100).toLocaleString('tr-TR', { maximumFractionDigits: 3 })}%` : '—';
        description.append(element('small', `${row.name} · ilvl ${row.required_ilvl} · w ${row.weight} · ${share}`));
        const button = element('button', active ? '✓' : existing ? '↔' : '+');
        const reason = manualReason(existing ? removeMod(item, existingIndex) : item, row);
        button.disabled = active || Boolean(reason);
        button.title = active ? 'Bu tier eşyada' : reason || (existing ? 'Bu tier ile değiştir; roll yeniden belirlenir' : 'Bu tier’ı ekle');
        button.setAttribute('aria-label', `${row.name} T${row.tier} ${active ? 'eşyada' : existing ? 'ile değiştir' : 'ekle'}`);
        button.onclick = () => attempt(() => commit(existing ? replaceTier(item, existingIndex, row) : manualAdd(item, row),
          existing ? `${existing.name} T${existing.tier}, ${row.name} T${row.tier} ile değiştirildi.` : `${row.name} T${row.tier} elle eklendi.`));
        entry.append(element('span', `T${row.tier}`, 'tier-id'), description, button); familyList.append(entry);
      }
      details.append(summary, familyList); list.append(details);
    }
    if (!list.children.length) list.append(element('p', 'Bu filtreyle mod bulunamadı.', 'empty-pool'));
  }
}

function render() { if (ready) {
  $('basic-controls').hidden = mode !== 'basic'; $('special-controls').hidden = mode === 'basic';
  document.querySelectorAll('[data-tier]').forEach(b => b.setAttribute('aria-pressed',String(b.dataset.tier === tier)));
  renderItem(); renderSpecials(); renderCurrencies(); renderMods();
} }

$('rarity').onchange = () => attempt(() => {
  const value = $('rarity').value;
  try { commit(setRarity(item, value), `Rarity ${value} seçildi.`); } finally { $('rarity').value = item.rarity; }
});
$('base').onchange = () => commit({ ...createItem($('base').value), ilvl: item.ilvl }, 'Taban türü değiştirildi; yeni craft başladı.',null,true);
$('ilvl').onchange = () => attempt(() => {
  const value = Number($('ilvl').value);
  if (!Number.isInteger(value) || value < 1 || value > 100) { $('ilvl').value = item.ilvl; throw new Error('Item Level 1–100 arasında tam sayı olmalı.'); }
  if (item.mods.some(m => m.required_ilvl > value)) { $('ilvl').value = item.ilvl; throw new Error('Bu seviye için fazla yüksek affix’leri önce kaldır.'); }
  commit({ ...item, ilvl: value }, `Item Level ${value} seçildi.`);
});
$('clear').onclick = () => commit(clearMods(item), `Affix’ler temizlendi; ${item.rarity} korundu.`);
$('reset').onclick = () => commit({ ...createItem(item.base), ilvl: item.ilvl }, 'Normal, boş eşya açıldı; işlem listesi ve maliyet sıfırlandı.',null,true);
$('undo').onclick = () => { const last = undo.pop(); if (!last) return; item = last.item; history = last.history; sessionStart = last.sessionStart; activeOmens = last.activeOmens; render(); status('Son işlem ve maliyeti geri alındı.'); persist(); };
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
  else if (!Number.isFinite(value) || value < 0) throw new Error('Birim fiyat sıfır veya pozitif olmalı.');
  else priceOverrides[specialSelected] = value;
  renderSpecials(); persist(); status('Birim fiyat sonraki kullanımlara uygulanır; geçmiş fiyatlar korunur.');
});
$('special-apply').onclick = () => attempt(() => {
  const rule = specials[specialSelected];
  const context = rule.operation;
  const effects = effectsFor(context,specialSelected,rule), payments = operationPayments(context,specialSelected,rule);
  commit(applySpecial(item,data(),rule,Math.random,effects),`${payments.map(p=>p.name).join(' + ')} kullanıldı.`,payments);
});
$('pool').onchange = () => { pool = $('pool').value; renderMods(); };
$('search').oninput = renderMods;
$('apply').onclick = () => attempt(() => {
  const rule = rules[selected];
  const effects = effectsFor('basic',selected,rule), payments = operationPayments('basic',selected,rule);
  commit(applyOrbOmens(item,data(),selected,rule,effects), `${payments.map(p=>p.name).join(' + ')} uygulandı.`,payments);
});
document.querySelectorAll('[data-tier]').forEach(button => button.onclick = () => {
  if (!ready) return;
  tier = button.dataset.tier;
  document.querySelectorAll('[data-tier]').forEach(b => b.setAttribute('aria-pressed', String(b === button)));
  selected = visibleRules().find(([id]) => kind(id) === kind(selected))?.[0] || visibleRules()[0][0];
  render(); status('Currency seviyesi değiştirildi.');
});
$('download').onclick = () => {
  const url = URL.createObjectURL(new Blob([JSON.stringify({ format: 2, item, history, archives, sessionStart }, null, 2)], { type: 'application/json' }));
  const link = element('a'); link.href = url; link.download = 'teorik-craft.json'; link.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000); status('Craft dosyası kaydedildi.');
};

async function readJSON(path) { const response = await fetch(path); if (!response.ok) throw new Error(`${path} yüklenemedi (${response.status}).`); return response.json(); }
try {
  const [loadedRules, manifest, specialData, loadedPrices, ...loadedData] = await Promise.all([
    readJSON('data/currency-rules.source.json'), readJSON('data/manifest.json'),
    readJSON('data/special-currencies.json'), Promise.resolve({currency:[],origin:'Uygulama fiyat listesi'}),
    ...Object.keys(labels).map(base => readJSON(`data/${base}.mods.json`)),
  ]);
  rules = loadedRules; Object.keys(labels).forEach((base, i) => { datasets[base] = loadedData[i]; });
  if (rules['greater-orb-of-augmentation']) rules['greater-orb-of-augmentation'].name = 'Greater Orb of Augmentation';
  specials = specialData.rules; prices = loadedPrices;
  omens = omenDefinitions(rules);
  if (omens['omen-of-the-liege']) omens['omen-of-the-liege'].icon = 'Art/2DItems/Currency/Omens/OmenOnAbyssGuarenteedLichTypeMod2.webp';
  let restored = false;
  try {
    const saved = JSON.parse(localStorage.getItem(storageKey));
    if (saved?.format === 2 && labels[saved.item?.base] && Array.isArray(saved.history) && Array.isArray(saved.item.mods)) {
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
  $('source-info').textContent = `Kaynak: PoE2DB · ${new Date(manifest.fetched_at_utc).toLocaleDateString('tr-TR', { timeZone: 'Europe/Istanbul' })} · yerel veri paketi`;
  ready = true;
  for (const id of ['base', 'ilvl', 'rarity', 'search', 'pool', 'download', 'reset']) $(id).disabled = false;
  $('workbench').setAttribute('aria-busy', 'false'); render();
  status(restored ? 'Kaydedilmiş craft ve maliyet geçmişi geri yüklendi.' : 'Eşya hazır. Rarity seçebilir, listeden affix ekleyebilir veya currency kullanabilirsin.');
} catch (error) {
  $('workbench').setAttribute('aria-busy', 'false'); status(`Veriler yüklenemedi. Uygulama veri paketini kontrol et. ${error.message}`, 'error');
  $('item-mods').replaceChildren(element('p', 'Mod verileri yüklenemedi.', 'empty-item'));
}

const parentOrigin = location.origin;
$('price-check').onclick = () => {
  try {
    $('price-check').disabled = true;
    parent.postMessage({type:'craft-price',raw:craftText(item)},parentOrigin);
  } catch (error) { status(error.message,'error'); renderItem(); }
};
window.addEventListener('message',event => {
  if (event.source !== parent || event.origin !== parentOrigin) return;
  if (event.data?.type === 'craft-prices' && Array.isArray(event.data.prices?.currency)) {
    prices = {...event.data.prices,origin:'Uygulama fiyat listesi'};
    render();
  } else if (event.data?.type === 'craft-result') {
    status(event.data.error || 'Craft pazar penceresine gönderildi.',event.data.error ? 'error' : 'success');
    renderItem();
  }
});
parent.postMessage({type:'craft-ready'},parentOrigin);
