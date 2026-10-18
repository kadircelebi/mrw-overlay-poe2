<script lang="ts">
  // The tracker: an ordinary window meant to stay open beside the game. Its
  // tabs grow one by one: the account's merchant sales, and the Expedition
  // rewards chosen map by map.
  import { onDestroy, onMount } from 'svelte'
  import { Events, Window } from '@wailsio/runtime'
  import { AppService } from '../bindings/poe2filter/internal/app'
  import type { ExpeditionEntry, ExpeditionHistoryView, TradeHistoryView, TradeSale } from '../bindings/poe2filter/internal/app/models'
  import type { Settings } from '../bindings/poe2filter/internal/overlay/models'
  import type { Total } from '../bindings/poe2filter/internal/tradehistory/models'
  import { t, locale } from './lib/i18n.svelte'
  import { followAppLanguage } from './lib/windowLang'
  import { clock, money, relative, until } from './lib/format'
  import { currencyInfo } from './lib/currencies.svelte'
  import Toggle from './lib/Toggle.svelte'

  type Tab = 'sales' | 'expedition'
  let tab = $state<Tab>('sales')
  let view = $state<TradeHistoryView | null>(null)
  let exp = $state<ExpeditionHistoryView | null>(null)
  // When the view arrived, for the quota countdown.
  let viewAt = $state(Date.now())
  let now = $state(Date.now())
  let query = $state('')
  let busy = $state(false)
  let settings = $state<Settings | null>(null)

  function accept(next: TradeHistoryView) {
    view = next
    viewAt = Date.now()
  }

  let timer: ReturnType<typeof setInterval> | undefined
  onMount(() => {
    AppService.GetTradeHistory().then(accept).catch(() => {})
    AppService.GetExpeditionHistory().then((v) => (exp = v)).catch(() => {})
    AppService.GetOverlaySettings().then((s) => (settings = s)).catch(() => {})
    const off = Events.On('trade-history', (event) => accept(event.data as TradeHistoryView))
    const offExp = Events.On('expedition-history', (event) => (exp = event.data as ExpeditionHistoryView))
    const offLang = followAppLanguage()
    timer = setInterval(() => (now = Date.now()), 1000)
    return () => {
      off()
      offExp()
      offLang()
    }
  })
  onDestroy(() => clearInterval(timer))

  const nextAt = $derived(view ? viewAt + view.nextInSec * 1000 : 0)
  const canRefresh = $derived(!!view && view.enabled && view.signedIn && !view.fetching && !busy && nextAt <= now)

  async function refresh() {
    busy = true
    try {
      accept(await AppService.RefreshTradeHistory())
    } finally {
      busy = false
    }
  }

  async function saveSettings(change: (s: Settings) => void) {
    const current = await AppService.GetOverlaySettings()
    change(current)
    settings = await AppService.SaveOverlaySettings(current)
  }

  async function clearAll() {
    if (!confirm(t('tracker.clearConfirm'))) return
    accept(await AppService.ClearTradeHistory())
  }

  async function resetMap() {
    if (!confirm(t('expedition.hist.resetConfirm'))) return
    exp = await AppService.ResetExpeditionMap()
  }

  async function deleteChoice(e: ExpeditionEntry) {
    exp = await AppService.DeleteExpeditionChoice(String(e.at))
  }

  async function clearExpedition() {
    if (!confirm(t('expedition.hist.clearConfirm'))) return
    exp = await AppService.ClearExpeditionHistory()
  }

  let expPlaceError = $state('')
  async function placeExpedition() {
    expPlaceError = (await AppService.PlaceExpeditionStrip()) ?? ''
  }

  async function setStrip(on: boolean) {
    exp = await AppService.SetExpeditionStrip(on)
    settings = await AppService.GetOverlaySettings()
  }

  function rewardName(e: ExpeditionEntry) {
    const name = e.display || e.name || e.text
    return e.countKnown && e.count > 1 ? `${e.count}× ${name}` : name
  }

  function expWorth(valueEx: number) {
    return valueEx > 0 ? money(valueEx, exp?.divineEx ?? 0) : '—'
  }

  // The history by day, and in a day by map: one group per map instance
  // (same id and seed), its choices newest first.
  type ExpMap = { key: string; map: string; area: string; level: number; entries: ExpeditionEntry[]; valueEx: number; open: boolean; from: number; to: number }
  type ExpDay = { key: string; label: string; maps: ExpMap[]; valueEx: number; count: number }
  const expDays = $derived.by(() => {
    const out: ExpDay[] = []
    for (const e of exp?.entries ?? []) {
      const d = new Date(e.at)
      const key = `${d.getFullYear()}-${d.getMonth()}-${d.getDate()}`
      let day = out[out.length - 1]
      if (!day || day.key !== key) {
        day = { key, label: d.toLocaleDateString(locale(), { weekday: 'long', day: 'numeric', month: 'long' }), maps: [], valueEx: 0, count: 0 }
        out.push(day)
      }
      const at = d.getTime()
      const mapKey = `${e.area}|${e.seed ?? ''}`
      let group = day.maps[day.maps.length - 1]
      if (!group || group.key !== mapKey) {
        group = { key: mapKey, map: e.map, area: e.area, level: e.level, entries: [], valueEx: 0, open: false, from: at, to: at }
        day.maps.push(group)
      }
      group.entries.push(e)
      group.valueEx += e.valueEx
      group.open ||= !e.done
      group.from = Math.min(group.from, at)
      group.to = Math.max(group.to, at)
      day.valueEx += e.valueEx
      day.count++
    }
    return out
  })

  // Which map groups are folded open: the current map unless closed, the
  // others only when opened.
  let unfolded = $state<Record<string, boolean>>({})
  function isUnfolded(g: ExpMap) {
    return unfolded[g.key] ?? g.open
  }
  function toggleMap(g: ExpMap) {
    unfolded[g.key] = !isUnfolded(g)
  }

  // Placing the overlay over the game: "" or "noGame" from the last try.
  let placeError = $state('')
  async function place() {
    placeError = (await AppService.PlaceEarnings()) ?? ''
  }

  function amount(total: Total | undefined) {
    if (!total || !view) return '—'
    return total.valueEx > 0 ? money(total.valueEx, view.divineEx) : '0'
  }

  function unpriced(total: Total | undefined) {
    const list = Object.entries(total?.unpriced ?? {}).filter(([, n]) => (n ?? 0) > 0)
    return list.map(([id, n]) => `${n} ${currencyInfo(id)?.text ?? id}`).join(', ')
  }

  function itemName(s: TradeSale) {
    const it = s.item
    return it.name ? `${it.name}, ${it.typeLine}` : it.typeLine
  }

  // The trade site draws item images on first request (gen/image); with a
  // whole list asking at once some fail. A failed one is asked again a few
  // times, then left as an empty slot rather than a broken-image mark.
  function retryIcon(event: Event) {
    const img = event.currentTarget as HTMLImageElement
    const tries = Number(img.dataset.tries ?? 0)
    if (tries >= 3) {
      img.classList.add('gone')
      return
    }
    img.dataset.tries = String(tries + 1)
    const src = img.dataset.src ?? img.src
    img.dataset.src = src
    setTimeout(() => (img.src = `${src}${src.includes('?') ? '&' : '?'}retry=${tries + 1}`), 2000 * 2 ** tries)
  }

  function rarityClass(s: TradeSale) {
    return (s.item.rarity ?? 'currency').toLowerCase()
  }

  function tooltip(s: TradeSale) {
    const lines = [itemName(s)]
    if (s.item.ilvl) lines.push(`${t('tracker.itemLevel')}: ${s.item.ilvl}`)
    lines.push(...(s.item.mods ?? []))
    if (s.item.corrupted) lines.push('Corrupted')
    return lines.join('\n')
  }

  type Day = { key: string; label: string; sales: TradeSale[]; valueEx: number }
  const days = $derived.by(() => {
    const out: Day[] = []
    const q = query.trim().toLocaleLowerCase()
    for (const s of view?.sales ?? []) {
      if (q && !itemName(s).toLocaleLowerCase().includes(q)) continue
      const d = new Date(s.time)
      const key = `${d.getFullYear()}-${d.getMonth()}-${d.getDate()}`
      let day = out[out.length - 1]
      if (!day || day.key !== key) {
        day = { key, label: d.toLocaleDateString(locale(), { weekday: 'long', day: 'numeric', month: 'long' }), sales: [], valueEx: 0 }
        out.push(day)
      }
      day.sales.push(s)
      day.valueEx += s.valueEx
    }
    return out
  })
</script>

<main class="tracker-win">
  <header class="titlebar">
    <img src="/emblem.png" alt="" class="emblem" />
    <h1 lang="en">{t('app.title')}</h1>
    <span class="crumb">{t('tracker.title')}</span>
    <span class="grow"></span>
    <button class="wbtn" title={t('window.minimise')} aria-label={t('window.minimise')} onclick={() => Window.Minimise()}>
      <svg viewBox="0 0 24 24"><path d="M6 12h12" /></svg>
    </button>
    <button class="wbtn" title={t('window.maximise')} aria-label={t('window.maximise')} onclick={() => Window.ToggleMaximise()}>
      <svg viewBox="0 0 24 24"><rect x="6" y="6" width="12" height="12" /></svg>
    </button>
    <button class="wbtn close" title={t('window.close')} aria-label={t('window.close')} onclick={() => Window.Close()}>
      <svg viewBox="0 0 24 24"><path d="M6 6l12 12M18 6L6 18" /></svg>
    </button>
  </header>

  <nav class="tabs" aria-label={t('tracker.title')}>
    <button class:on={tab === 'sales'} onclick={() => (tab = 'sales')}>{t('tracker.tab.sales')}</button>
    <button class:on={tab === 'expedition'} onclick={() => (tab = 'expedition')}>{t('tracker.tab.expedition')}</button>
  </nav>

  <section class="body">
    {#if tab === 'expedition'}
      {#if !exp}
        <p class="muted">…</p>
      {:else}
        {#if settings && !settings.enabled}
          <div class="card warn">
            <p>{t('expedition.hist.needOverlay')}</p>
            <button onclick={() => AppService.ShowSettings('')}>{t('header.settings')}</button>
          </div>
        {/if}
        <div class="card overlay-row">
          <p class="muted small">{t('expedition.hist.intro')}</p>
          <Toggle checked={exp.strip} label={t('expedition.hist.strip')} hint={t('expedition.hist.stripHint')} onchange={(v) => setStrip(v)} />
          {#if exp.strip}
            <div class="sub-toggle">
              <Toggle checked={exp.names} label={t('expedition.hist.names')} hint={t('expedition.hist.namesHint')} onchange={(v) => saveSettings((s) => (s.expedition_strip_names = v))} />
            </div>
            <div class="place">
              {#if exp.placing}
                <span class="muted small">{t('tracker.placeHint')}</span>
                <button class="primary" onclick={() => AppService.FinishExpeditionPlace(true)}>{t('tracker.placeSave')}</button>
                <button onclick={() => AppService.FinishExpeditionPlace(false)}>{t('tracker.placeCancel')}</button>
              {:else}
                <button onclick={placeExpedition}>{t('tracker.place')}</button>
                {#if expPlaceError === 'noGame'}<span class="muted small">{t('tracker.placeNoGame')}</span>{/if}
              {/if}
            </div>
          {/if}
        </div>

        {#if exp.pending}
          <div class="card pending">
            <span class="muted small">{t('expedition.hist.current', exp.pending.map)}</span>
            <span class="pending-reward">{t('expedition.hist.maps', exp.mapTotal.maps)} · {rewardName(exp.pending)}</span>
            <span class="value num">{expWorth(exp.mapTotal.valueEx)}</span>
            <button class="small-btn" onclick={resetMap}>{t('expedition.hist.reset')}</button>
          </div>
        {/if}

        <div class="totals">
          <div class="total">
            <span class="label">{t('tracker.session')}</span>
            <span class="value num">{expWorth(exp.session.valueEx)}</span>
            <span class="sub">{t('expedition.hist.maps', exp.session.maps)} · {t('tracker.since', clock(exp.sessionStart))}</span>
          </div>
          <div class="total">
            <span class="label">{t('tracker.today')}</span>
            <span class="value num">{expWorth(exp.today.valueEx)}</span>
            <span class="sub">{t('expedition.hist.maps', exp.today.maps)}</span>
          </div>
          <div class="total">
            <span class="label">{t('tracker.league')}</span>
            <span class="value num">{expWorth(exp.leagueTotal.valueEx)}</span>
            <span class="sub">{t('expedition.hist.maps', exp.leagueTotal.maps)} · {exp.league}</span>
          </div>
        </div>

        {#if expDays.length === 0}
          <p class="muted empty">{t('expedition.hist.empty')}</p>
        {:else}
          <div class="list">
            {#each expDays as day (day.key)}
              <div class="day">
                <span>{day.label}</span>
                <span class="num">{t('expedition.hist.maps', day.count)} · {expWorth(day.valueEx)}</span>
              </div>
              {#each day.maps as g (g.key + g.from)}
                <button class="map-head" class:open={g.open} aria-expanded={isUnfolded(g)} onclick={() => toggleMap(g)}>
                  <span class="chev">{isUnfolded(g) ? '▾' : '▸'}</span>
                  <span class="time num">{clock(g.from)}{g.to !== g.from ? `–${clock(g.to)}` : ''}</span>
                  <span class="map" title={g.area}>{g.map}{#if g.level}&nbsp;<span class="muted">({g.level})</span>{/if}</span>
                  <span class="count muted">{t('expedition.hist.maps', g.entries.length)}{#if g.open}&nbsp;·&nbsp;<span class="open-label">{t('expedition.hist.inProgress')}</span>{/if}</span>
                  <span class="worth num">{expWorth(g.valueEx)}</span>
                </button>
                {#if isUnfolded(g)}
                  {#each g.entries as e (e.at)}
                    <div class="row exp-row" class:open={!e.done}>
                      <span class="time num">{clock(new Date(e.at).getTime())}</span>
                      <span class="name currency">{rewardName(e)}</span>
                      <span class="worth num">{e.valueEx > 0 ? `≈ ${expWorth(e.valueEx)}` : ''}</span>
                      <button class="del" title={t('expedition.hist.delete')} aria-label={t('expedition.hist.delete')} onclick={() => deleteChoice(e)}>✕</button>
                    </div>
                  {/each}
                {/if}
              {/each}
            {/each}
          </div>
        {/if}

        <div class="card options">
          <p class="muted small">{t('expedition.hist.note')}</p>
          <div class="end">
            <button class="link" onclick={clearExpedition}>{t('expedition.hist.clear')}</button>
          </div>
        </div>
      {/if}
    {:else if !view}
      <p class="muted">…</p>
    {:else if !view.enabled}
      <div class="card intro">
        <h2>{t('tracker.off.title')}</h2>
        <p>{t('tracker.off.text')}</p>
        <button class="primary" onclick={() => saveSettings((s) => (s.trade_history = true))}>{t('tracker.off.turnOn')}</button>
      </div>
    {:else}
      {#if !view.signedIn || view.status === 'signedOut'}
        <div class="card warn">
          <p>{t('tracker.signedOut')}</p>
          <button onclick={() => AppService.ShowSettings('account')}>{t('tracker.connect')}</button>
        </div>
      {/if}

      {#if settings}
        <div class="card overlay-row">
          <Toggle
            checked={settings.earnings_overlay ?? false}
            label={t('tracker.overlay')}
            hint={t('tracker.overlayHint')}
            onchange={(v) => saveSettings((s) => (s.earnings_overlay = v))}
          />
          {#if settings.earnings_overlay}
            <div class="place">
              {#if view.placing}
                <span class="muted small">{t('tracker.placeHint')}</span>
                <button class="primary" onclick={() => AppService.FinishEarningsPlace(true)}>{t('tracker.placeSave')}</button>
                <button onclick={() => AppService.FinishEarningsPlace(false)}>{t('tracker.placeCancel')}</button>
              {:else}
                <button onclick={place}>{t('tracker.place')}</button>
                {#if placeError === 'noGame'}<span class="muted small">{t('tracker.placeNoGame')}</span>{/if}
              {/if}
            </div>
          {/if}
        </div>
      {/if}

      <div class="totals">
        <div class="total">
          <span class="label">{t('tracker.session')}</span>
          <span class="value num">{amount(view.session)}</span>
          <span class="sub">{t('tracker.sales', view.session.count)} · {t('tracker.since', clock(view.sessionStart))}</span>
        </div>
        <div class="total">
          <span class="label">{t('tracker.today')}</span>
          <span class="value num">{amount(view.today)}</span>
          <span class="sub">{t('tracker.sales', view.today.count)}</span>
        </div>
        <div class="total">
          <span class="label">{t('tracker.league')}</span>
          <span class="value num">{amount(view.leagueTotal)}</span>
          <span class="sub">{t('tracker.sales', view.leagueTotal.count)} · {view.league}</span>
        </div>
      </div>
      {#if unpriced(view.leagueTotal)}
        <p class="muted small">{t('tracker.unpriced', unpriced(view.leagueTotal))}</p>
      {/if}

      <div class="toolbar">
        <input type="search" placeholder={t('tracker.search')} bind:value={query} />
        <span class="status">
          {#if view.fetching || busy}
            {t('tracker.fetching')}
          {:else if view.status === 'error'}
            <span class="bad" title={view.error}>{t('tracker.error')}</span>
          {:else if view.fetchedAt}
            {t('tracker.updated', relative(view.fetchedAt, now))}
          {:else}
            {t('tracker.never')}
          {/if}
        </span>
        <button onclick={refresh} disabled={!canRefresh}>
          {nextAt > now && !view.fetching ? t('tracker.refreshIn', until(nextAt, now)) : t('tracker.refresh')}
        </button>
      </div>

      {#if days.length === 0}
        <p class="muted empty">{query ? t('tracker.noMatch') : t('tracker.empty')}</p>
      {:else}
        <div class="list">
          {#each days as day (day.key)}
            <div class="day">
              <span>{day.label}</span>
              <span class="num">{t('tracker.sales', day.sales.length)} · {money(day.valueEx, view.divineEx)}</span>
            </div>
            {#each day.sales as s (s.itemId + s.time)}
              <div class="row" title={tooltip(s)}>
                <span class="time num">{clock(new Date(s.time).getTime())}</span>
                {#if s.item.icon}<img class="icon" src={s.item.icon} alt="" loading="lazy" onerror={retryIcon} />{:else}<span class="icon"></span>{/if}
                <span class="name {rarityClass(s)}">{itemName(s)}</span>
                <span class="price num">
                  {s.amount}
                  {#if currencyInfo(s.currency)?.image}
                    <img class="cur" src={currencyInfo(s.currency)?.image} alt={currencyInfo(s.currency)?.text ?? s.currency} title={currencyInfo(s.currency)?.text} />
                  {:else}
                    {s.currency}
                  {/if}
                </span>
                <span class="worth num">{s.valueEx > 0 ? `≈ ${money(s.valueEx, view.divineEx)}` : ''}</span>
              </div>
            {/each}
          {/each}
        </div>
      {/if}

      <div class="card options">
        <p class="muted small">
          {t('tracker.note', view.divineEx > 0 ? view.divineEx.toFixed(0) : '?')}
        </p>
        <div class="end">
          <button class="link" onclick={() => saveSettings((s) => (s.trade_history = false))}>{t('tracker.turnOff')}</button>
          <button class="link" onclick={clearAll}>{t('tracker.clear')}</button>
        </div>
      </div>
    {/if}
  </section>
</main>

<style>
  main.tracker-win {
    height: 100%;
    display: flex;
    flex-direction: column;
    background:
      linear-gradient(180deg, var(--ui-tint, rgba(255, 255, 255, 0.03)), transparent 260px),
      var(--grain),
      var(--bg);
    border: 1px solid var(--line-strong);
    overflow: hidden;
  }
  .titlebar {
    flex: none;
    display: flex;
    align-items: center;
    gap: 10px;
    height: 40px;
    padding: 0 4px 0 12px;
    border-bottom: 1px solid var(--line-strong);
    background: linear-gradient(var(--ui-surface-2, #1b1f25), var(--ui-surface, #121518));
    --wails-draggable: drag;
  }
  .titlebar button {
    --wails-draggable: no-drag;
  }
  .emblem {
    width: 22px;
    height: 22px;
  }
  h1 {
    margin: 0;
    font-family: var(--serif);
    font-weight: 600;
    font-size: 12.5px;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    white-space: nowrap;
    color: var(--gold-bright);
  }
  .crumb {
    color: var(--muted);
    font-size: 12px;
  }
  .grow {
    flex: 1;
  }
  .wbtn {
    width: 38px;
    height: 30px;
    display: grid;
    place-items: center;
    border: 0;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-2);
  }
  .wbtn:hover {
    background: var(--surface-3);
    color: var(--text);
  }
  .wbtn.close:hover {
    background: var(--ui-hover, #8a2c27);
    color: var(--ui-text, #fff);
  }
  .wbtn svg {
    width: 14px;
    height: 14px;
    fill: none;
    stroke: currentColor;
    stroke-width: 1.8;
  }
  .tabs {
    flex: none;
    display: flex;
    gap: 2px;
    padding: 8px 14px 0;
    border-bottom: 1px solid var(--line);
  }
  .tabs button {
    border: 0;
    border-bottom: 2px solid transparent;
    background: transparent;
    color: var(--text-2);
    padding: 6px 12px 8px;
    font-size: 13px;
  }
  .tabs button.on {
    color: var(--gold-bright);
    border-bottom-color: var(--gold);
  }
  .body {
    flex: 1;
    min-height: 0;
    overflow: auto;
    padding: 14px 16px 18px;
    display: flex;
    flex-direction: column;
    gap: 12px;
  }
  .card {
    border: 1px solid var(--line);
    border-radius: var(--radius-sm);
    background: var(--surface);
    padding: 12px 14px;
  }
  .card p {
    margin: 0 0 10px;
    line-height: 1.5;
  }
  .card.intro {
    max-width: 640px;
  }
  .card.intro h2 {
    margin: 0 0 8px;
    font-family: var(--serif);
    font-size: 15px;
    color: var(--gold-bright);
  }
  .card.warn {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    border-color: var(--warn);
  }
  .card.warn p {
    margin: 0;
  }
  button {
    border: 1px solid var(--line-strong);
    border-radius: var(--radius-sm);
    background: var(--surface-2);
    color: var(--text);
    padding: 6px 12px;
  }
  button:hover:not(:disabled) {
    background: var(--surface-3);
  }
  button:disabled {
    opacity: 0.55;
  }
  button.primary {
    border-color: var(--gold-dim);
    color: var(--gold-bright);
  }
  button.link {
    border: 0;
    background: transparent;
    color: var(--muted);
    padding: 2px 4px;
    text-decoration: underline;
  }
  .totals {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 10px;
  }
  .total {
    display: flex;
    flex-direction: column;
    gap: 4px;
    border: 1px solid var(--line);
    border-radius: var(--radius-sm);
    background: var(--surface);
    padding: 12px 14px;
  }
  .total .label {
    color: var(--muted);
    font-size: 12px;
  }
  .total .value {
    font-size: 22px;
    color: var(--gold-bright);
  }
  .total .sub {
    color: var(--text-2);
    font-size: 12px;
  }
  .toolbar {
    display: flex;
    align-items: center;
    gap: 10px;
  }
  .toolbar input {
    flex: 1;
    max-width: 320px;
  }
  .toolbar .status {
    flex: 1;
    color: var(--muted);
    font-size: 12px;
    text-align: right;
  }
  .bad {
    color: var(--bad);
  }
  .muted {
    color: var(--muted);
  }
  .small {
    font-size: 12px;
  }
  .empty {
    padding: 24px 0;
    text-align: center;
  }
  .list {
    display: flex;
    flex-direction: column;
    border: 1px solid var(--line);
    border-radius: var(--radius-sm);
    background: var(--sunk);
  }
  .day {
    display: flex;
    justify-content: space-between;
    padding: 8px 12px;
    background: var(--surface-2);
    border-bottom: 1px solid var(--line);
    color: var(--text-2);
    font-size: 12px;
  }
  .row {
    display: grid;
    grid-template-columns: 48px 28px 1fr auto 90px;
    align-items: center;
    gap: 10px;
    padding: 5px 12px;
    border-bottom: 1px solid var(--line);
  }
  .row:last-child {
    border-bottom: 0;
  }
  .row:hover {
    background: var(--surface);
  }
  .time {
    color: var(--muted);
    font-size: 12px;
  }
  .icon {
    width: 28px;
    height: 28px;
    object-fit: contain;
  }
  .icon:global(.gone) {
    visibility: hidden;
  }
  .name {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .name.unique {
    color: #af6025;
  }
  .name.rare {
    color: #ffff77;
  }
  .name.magic {
    color: #8888ff;
  }
  .name.normal {
    color: #c8c8c8;
  }
  .name.currency {
    color: #aa9e82;
  }
  .price {
    display: flex;
    align-items: center;
    gap: 4px;
  }
  .cur {
    width: 20px;
    height: 20px;
  }
  .worth {
    color: var(--text-2);
    font-size: 12px;
    text-align: right;
  }
  .options {
    display: flex;
    flex-direction: column;
    gap: 10px;
  }
  .options p {
    margin: 0;
  }
  .overlay-row {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .place {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 10px;
  }
  .end {
    display: flex;
    gap: 12px;
  }
  .card.pending {
    display: grid;
    grid-template-columns: auto 1fr auto auto;
    align-items: center;
    gap: 12px;
    border-color: var(--gold-dim);
  }
  .pending-reward {
    color: var(--gold-bright);
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .card.pending .value {
    color: var(--gold-bright);
  }
  .row.exp-row {
    grid-template-columns: 48px 1fr 110px 24px;
    padding-left: 44px;
  }
  .row.exp-row .del {
    width: 24px;
    height: 22px;
    padding: 0;
    border: 0;
    background: transparent;
    color: var(--muted);
    visibility: hidden;
  }
  .row.exp-row:hover .del {
    visibility: visible;
  }
  .row.exp-row .del:hover {
    color: var(--bad);
  }
  .small-btn {
    padding: 3px 10px;
    font-size: 12px;
  }
  .map-head {
    display: grid;
    grid-template-columns: 16px 96px minmax(120px, 1fr) auto 110px;
    align-items: center;
    gap: 10px;
    width: 100%;
    padding: 7px 12px;
    border: 0;
    border-bottom: 1px solid var(--line);
    border-radius: 0;
    background: var(--surface);
    color: var(--text);
    text-align: left;
  }
  .map-head:hover:not(:disabled) {
    background: var(--surface-2);
  }
  .map-head .chev {
    color: var(--muted);
  }
  .map-head .count {
    font-size: 12px;
  }
  .map-head .worth {
    color: var(--text);
  }
  .map-head.open .worth,
  .open-label {
    color: var(--gold);
  }
  .row.exp-row.open .worth {
    color: var(--gold);
  }
  .sub-toggle {
    padding-left: 18px;
    border-left: 2px solid var(--line);
  }
  .map {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--text-2);
  }
</style>
