<script lang="ts">
  import { onMount } from 'svelte'
  import { Events } from '@wailsio/runtime'
  import { AppService, type Meta } from '../bindings/poe2filter'
  import type { Config } from '../bindings/poe2filter/internal/filter/models'
  import type { State } from '../bindings/poe2filter/internal/engine/models'
  import Toggle from './lib/Toggle.svelte'
  import Segmented from './lib/Segmented.svelte'
  import ListEditor from './lib/ListEditor.svelte'
  import StylePreview from './lib/StylePreview.svelte'
  import type { StyleGroup, Theme } from '../bindings/poe2filter/internal/filter/models'
  import { clock, relative, until, money, strictnessNames } from './lib/format'

  let meta = $state<Meta | null>(null)
  let cfg = $state<Config | null>(null)
  let st = $state<State | null>(null)
  let view = $state<'main' | 'settings'>('main')
  let now = $state(Date.now())
  // Settings changed since the filter was last written.
  let dirty = $state(false)
  let saveState = $state<'idle' | 'saving' | 'saved' | 'error'>('idle')
  let saveTimer: ReturnType<typeof setTimeout> | undefined
  let saveSeq = 0
  let actionError = $state('')
  let themes = $state<Theme[]>([])
  let groups = $state<StyleGroup[]>([])
  let styleGroup = $state('divine')
  let sounds = $state<string[]>([])
  let soundError = $state('')
  let leagues = $state<string[]>([])

  async function refresh() {
    st = await AppService.GetState()
    leagues = (await AppService.Leagues()) ?? leagues
  }

  onMount(() => {
    ;(async () => {
      meta = await AppService.GetMeta()
      themes = (await AppService.Themes()) ?? []
      groups = (await AppService.StyleGroups()) ?? []
      sounds = (await AppService.ListSounds()) ?? []
      leagues = (await AppService.Leagues()) ?? []
      cfg = await AppService.GetConfig()
      await refresh()
    })()
    const off = Events.On('state', (ev) => {
      const prevRunning = st?.running
      st = ev.data
      if (prevRunning && !st.running && !st.lastError) dirty = false
    })
    const tick = setInterval(() => (now = Date.now()), 1000)
    window.addEventListener('focus', refresh)
    return () => {
      off()
      clearInterval(tick)
      window.removeEventListener('focus', refresh)
    }
  })

  // Save shortly after the last change; the filter is rebuilt only on demand.
  function queueSave(affectsFilter = true) {
    if (affectsFilter) dirty = true
    saveState = 'saving'
    saveSeq++ // invalidates any reply still in flight
    clearTimeout(saveTimer)
    saveTimer = setTimeout(async () => {
      const seq = ++saveSeq
      try {
        const saved = await AppService.SaveConfig($state.snapshot(cfg) as Config)
        // Ignore stale replies: the user may have kept editing meanwhile.
        if (seq !== saveSeq) return
        cfg = saved
        saveState = 'saved'
        setTimeout(() => saveState === 'saved' && (saveState = 'idle'), 1500)
      } catch (e) {
        saveState = 'error'
      }
    }, 450)
  }

  async function updateNow() {
    actionError = ''
    try {
      await AppService.UpdateNow()
    } catch (e) {
      actionError = String(e)
    }
  }

  const last = $derived(st?.last ?? null)
  const divineEx = $derived(last?.divineEx ?? 0)
  const chaosEx = $derived(last?.chaosEx ?? 0)

  // Live conversion of the threshold, shown under the input.
  const thresholdEx = $derived.by(() => {
    if (!cfg) return 0
    if (cfg.min_value_unit === 'divine') return cfg.min_value * divineEx
    if (cfg.min_value_unit === 'chaos') return cfg.min_value * chaosEx
    return cfg.min_value
  })

  const status = $derived.by(() => {
    if (!st) return { tone: 'idle', title: 'Başlatılıyor…', sub: '' }
    if (st.running) return { tone: 'busy', title: 'Güncelleniyor', sub: st.step }
    if (st.lastError) return { tone: 'bad', title: 'Filtre güncellenemedi', sub: st.lastError }
    if (st.lastRunAtMs) return { tone: 'ok', title: 'Filtre güncel', sub: `Son güncelleme ${relative(st.lastRunAtMs, now)}` }
    return { tone: 'idle', title: 'Henüz güncellenmedi', sub: '' }
  })

  // The configured league is always offered, even if the live list lost it.
  const leagueOptions = $derived.by(() => {
    const list = leagues.length ? [...leagues] : []
    const current = cfg?.league_name ?? ''
    if (current && !list.some((l) => l.toLowerCase() === current.toLowerCase())) list.push(current)
    return list
  })
  const leagueUnlisted = $derived(
    !!cfg?.league_name &&
      leagues.length > 0 &&
      !leagues.some((l) => l.toLowerCase() === cfg!.league_name.toLowerCase()),
  )

  const selGroup = $derived(groups.find((g) => g.id === styleGroup))

  // Groups without a built-in look (Divine Orb) default to their first theme.
  function styleValue(g: StyleGroup): string {
    const v = cfg?.styles?.[g.id]
    if (v) return v
    return g.allowDefault ? 'default' : g.default.id
  }

  function paletteOf(g: StyleGroup): Theme {
    return themes.find((t) => t.id === cfg?.styles?.[g.id]) ?? g.default
  }

  function customised(g: StyleGroup): boolean {
    const v = styleValue(g)
    return g.allowDefault ? v !== 'default' : v !== g.default.id
  }

  function setSound(group: string, v: string) {
    if (!cfg) return
    const next = { ...(cfg.sounds ?? {}) }
    if (v) next[group] = v
    else delete next[group]
    cfg.sounds = next
    soundError = ''
    queueSave()
  }

  function soundFile(g: StyleGroup): string {
    const v = cfg?.sounds?.[g.id] ?? ''
    return v.startsWith('file:') ? v.slice(5) : ''
  }

  function soundLabel(g: StyleGroup): string {
    const v = cfg?.sounds?.[g.id] ?? ''
    if (v === 'none') return 'sessiz'
    if (v.startsWith('file:')) return v.slice(5)
    if (v) return `oyun sesi ${v}`
    return g.defaultSound ? `oyun sesi ${g.defaultSound}` : 'sessiz'
  }

  async function previewSound(g: StyleGroup) {
    soundError = ''
    try {
      await AppService.PreviewSound(soundFile(g))
    } catch (e) {
      soundError = String(e)
    }
  }

  function setStyle(group: string, id: string) {
    if (!cfg) return
    cfg.styles = { ...(cfg.styles ?? {}), [group]: id }
    queueSave()
  }

  const scanPct = $derived(st && st.scan.keys ? Math.min(1, st.scan.scanned / st.scan.keys) : 0)
</script>

<main>
  <header>
    <img src="/emblem.png" alt="" class="emblem" />
    {#if view === 'main'}
      <div class="brand">
        <h1>PoE2 Filtre</h1>
        {#if cfg}<span class="league">{cfg.league_name}</span>{/if}
      </div>
      <button class="icon" title="Ayarlar" aria-label="Ayarlar" onclick={() => (view = 'settings')}>
        <svg viewBox="0 0 24 24"><path d="M12 15.5a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7Z" /><path d="M19.4 15a1.7 1.7 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.7 1.7 0 0 0-1.8-.3 1.7 1.7 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.7 1.7 0 0 0-1.1-1.5 1.7 1.7 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.7 1.7 0 0 0 .3-1.8 1.7 1.7 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.7 1.7 0 0 0 1.5-1.1 1.7 1.7 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.7 1.7 0 0 0 1.8.3H9a1.7 1.7 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.7 1.7 0 0 0 1 1.5 1.7 1.7 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.7 1.7 0 0 0-.3 1.8V9a1.7 1.7 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.7 1.7 0 0 0-1.5 1Z" /></svg>
      </button>
    {:else}
      <div class="brand">
        <h1>Ayarlar</h1>
        <span class="save {saveState}">
          {#if saveState === 'saving'}kaydediliyor…{:else if saveState === 'saved'}kaydedildi{:else if saveState === 'error'}kaydedilemedi{/if}
        </span>
      </div>
      <button class="icon" title="Geri" aria-label="Geri" onclick={() => (view = 'main')}>
        <svg viewBox="0 0 24 24"><path d="M15 18l-6-6 6-6" /></svg>
      </button>
    {/if}
    <button class="icon" title="Gizle" aria-label="Paneli gizle" onclick={() => AppService.HidePanel()}>
      <svg viewBox="0 0 24 24"><path d="M6 6l12 12M18 6L6 18" /></svg>
    </button>
  </header>

  {#if cfg && view === 'main'}
    <div class="scroll">
      <!-- Status -->
      <section class="card status {status.tone}">
        <div class="status-head">
          <span class="dot"></span>
          <div class="status-text">
            <strong>{status.title}</strong>
            {#if status.sub}<span class="sub" title={status.sub}>{status.sub}</span>{/if}
          </div>
        </div>
        {#if st?.running}
          <div class="bar"><span style="width: {Math.round((st.progress || 0.05) * 100)}%"></span></div>
        {/if}
        {#if dirty && !st?.running}
          <p class="notice">Ayarlar değişti. Filtreye yansıması için güncelle.</p>
        {/if}
        {#if st?.lastError && !st.running}
          <p class="notice bad">
            Oyundaki filtre yazılamadı, {st.lastOkAtMs
              ? `dosya ${relative(st.lastOkAtMs, now)} yazılan hâliyle duruyor`
              : 'henüz hiç yazılmadı'}. Yeni ayarların oyuna yansıması için güncellemenin başarılı olması gerekir.
          </p>
        {/if}
        <button class="primary" class:pulse={dirty} disabled={st?.running} onclick={updateNow}>
          {st?.running ? 'Güncelleniyor…' : st?.lastError ? 'Tekrar dene' : 'Şimdi güncelle'}
        </button>
        <div class="meta-row">
          {#if st?.running}
            <span>&nbsp;</span>
          {:else if st?.nextRetryAtMs}
            <span>Otomatik tekrar: {clock(st.nextRetryAtMs)} <em>({until(st.nextRetryAtMs, now)})</em></span>
          {:else if st?.nextRunAtMs}
            <span>Sonraki: {clock(st.nextRunAtMs)} <em>({until(st.nextRunAtMs, now)})</em></span>
          {:else if !cfg.auto_update_enabled}
            <span>Otomatik güncelleme kapalı</span>
          {/if}
          <span class="reload">Oyunda: Item Filter → Reload</span>
        </div>
        {#if actionError}<p class="error">{actionError}</p>{/if}
      </section>

      <!-- Threshold -->
      <section class="card">
        <div class="card-title">
          <h2>Değer eşiği</h2>
          {#if divineEx && cfg.min_value_unit !== 'exalted'}<span class="aside num">≈ {money(thresholdEx, 0)}</span>{/if}
        </div>
        <p class="desc">Bu değerin altındaki eşyalar {cfg.filter_mode === 'dim' ? 'soluk gösterilir' : cfg.filter_mode === 'show_only' ? 'yine gösterilir' : 'gizlenir'}.</p>
        <div class="threshold">
          <input
            type="number"
            class="num"
            min="0"
            step={cfg.min_value_unit === 'divine' ? 0.1 : 1}
            bind:value={cfg.min_value}
            oninput={() => queueSave()}
          />
          <Segmented
            small
            bind:value={cfg.min_value_unit}
            onchange={() => queueSave()}
            options={[
              { value: 'exalted', label: 'Exalted' },
              { value: 'chaos', label: 'Chaos' },
              { value: 'divine', label: 'Divine' },
            ]}
          />
        </div>
        <div class="chips">
          {#each cfg.min_value_unit === 'divine' ? [0.1, 0.5, 1, 5] : [5, 10, 50, 100] as v}
            <button class:on={cfg.min_value === v} onclick={() => { cfg!.min_value = v; queueSave() }}>{v}</button>
          {/each}
        </div>
      </section>

      <!-- Strictness + mode -->
      <section class="card">
        <div class="card-title">
          <h2><span lang="en">NeverSink</span> temeli</h2>
          <span class="aside gold">{strictnessNames[cfg.strictness]}</span>
        </div>
        <input
          type="range"
          class="strict"
          min="0"
          max="6"
          step="1"
          bind:value={cfg.strictness}
          onchange={() => queueSave()}
          style="--p: {(cfg.strictness / 6) * 100}%"
        />
        <div class="scale"><span>Soft</span><span>Strict</span><span>Uber+</span></div>

        <h2 class="sub-title">Eşik altı eşyalar</h2>
        <Segmented
          bind:value={cfg.filter_mode}
          onchange={() => queueSave()}
          options={[
            { value: 'hide', label: 'Gizle' },
            { value: 'dim', label: 'Soluk' },
            { value: 'show_only', label: 'Göster' },
          ]}
        />
      </section>

      <!-- Exceptional scan -->
      <section class="card">
        <Toggle
          bind:checked={cfg.exceptional_scan}
          label="Exceptional taraması"
          hint="Fazladan soketli ve %21+ kaliteli tabanları trade'den fiyatlar"
          onchange={() => queueSave(false)}
        />
        {#if st && cfg.exceptional_scan}
          <div class="bar thin"><span class="cyan" style="width: {scanPct * 100}%"></span></div>
          <div class="scan-line num">
            <span>{st.scan.scanned} / {st.scan.keys || '—'} tarandı</span>
            <span class="cyan-text">{st.scan.valuable} değerli</span>
          </div>
          {#if st.scan.current}
            <div class="scan-line muted">
              <span class="ellipsis">Sırada: {st.scan.current}</span>
              <span class="num">{st.scan.nextAtMs > now + 1000 ? until(st.scan.nextAtMs, now) : 'aranıyor…'}</span>
            </div>
          {/if}
          {#if st.scan.last}
            <div class="scan-line muted"><span class="ellipsis">Son: {st.scan.last}</span></div>
          {/if}
          {#if st.scan.etaSec > 0}
            <p class="scan-note">
              Aramalar kotayı korumak için ~{Math.round(st.scan.etaSec / Math.max(1, st.scan.keys - st.scan.scanned))} sn arayla yapılıyor.
              Tüm tabanların ilk taraması ≈ {until(now + st.scan.etaSec * 1000, now)}; NeverSink'in değerli bulduğu tabanlar önce.
            </p>
          {/if}
        {/if}
      </section>

      <!-- Summary -->
      {#if last}
        <section class="stats">
          <div><strong class="num">{last.valuableCurrency}</strong><span>currency</span></div>
          <div><strong class="num">{last.valuableUniques}</strong><span>unique taban</span></div>
          <div><strong class="num cyan-text">{last.valuableExcept}</strong><span>exceptional</span></div>
        </section>
        <p class="stats-caption">eşiğin üstünde vurgulanan · 1 div = {Math.round(divineEx)} ex</p>
      {/if}
    </div>
  {:else if cfg && view === 'settings'}
    <div class="scroll settings">
      <section class="card">
        <h2>Ekipman</h2>
        <Toggle bind:checked={cfg.include_gear} label="Sıkı ekipman filtresi" hint="Sıradan silah ve zırhları gizler" onchange={() => queueSave()} />
        <Toggle bind:checked={cfg.t5_rares} label="Tier 5 rare ekipman" hint="Tanımlanmamış tier'ı 5 olan rare'ler hep görünür" onchange={() => queueSave()} />
        <Toggle bind:checked={cfg.t5_jewels_only} label="Rare jewel'larda sadece T5" onchange={() => queueSave()} />
        <label class="field">
          <span>Yüksek kalite ekipmanı göster</span>
          <select bind:value={cfg.quality_threshold} onchange={() => queueSave()}>
            <option value={0}>Kapalı</option>
            <option value={15}>%15+</option>
            <option value={20}>%20+</option>
          </select>
        </label>
      </section>

      <section class="card">
        <h2>Özel kurallar</h2>
        <Toggle bind:checked={cfg.high_waystones} label="T14+ waystone vurgusu" onchange={() => queueSave()} />
        <Toggle bind:checked={cfg.high_uncut_gems} label="Sadece 20. seviye uncut gem" hint="Diğer uncut gem'ler gizlenir" onchange={() => queueSave()} />
        <Toggle bind:checked={cfg.uncut_support_gems} label="Uncut support gem'leri göster" hint="Kapalıyken hepsi gizlenir; açıkken 20. seviye kuralına uyar" onchange={() => queueSave()} />
        <Toggle bind:checked={cfg.boss_keys_and_tablets} label="Pinnacle anahtarları vurgusu" onchange={() => queueSave()} />
        <Toggle bind:checked={cfg.hide_exalt} label="Exalted Orb'ları gizle" onchange={() => queueSave()} />
        <Toggle bind:checked={cfg.hide_gold} label="Gold'u gizle" onchange={() => queueSave()} />
      </section>

      <section class="card">
        <h2>Listeler</h2>
        <h3>Her zaman göster — öne çıkar <span class="h3-note">en güçlü vurgu</span></h3>
        <p class="desc">Değerli unique'i olan tabanlar (Mageblood, Headhunter, Voices…) zaten otomatik ve sadece unique olarak gösterilir.</p>
        <ListEditor bind:items={cfg.whitelist} uniqueVariants placeholder="Unique, currency veya taban ara…" onchange={() => queueSave()} />
        <h3>Her zaman göster — orta <span class="h3-note">asla gizlenmez, orta vurgu</span></h3>
        <p class="desc">Eşya zaten değerliyse güçlü vurgusunu korur; değilse bu orta seviye görünümle gösterilir.</p>
        <ListEditor bind:items={cfg.whitelist_mid} uniqueVariants placeholder="Unique, currency veya taban ara…" onchange={() => queueSave()} />
        <h3>Her zaman gizle</h3>
        <ListEditor bind:items={cfg.blacklist} placeholder="Gizlenecek eşya ara…" onchange={() => queueSave()} />
        <h3>Chance tabanları <span class="h3-note">sadece normal nadirlik</span></h3>
        <ListEditor bind:items={cfg.chance_bases} placeholder="Taban veya unique ara…" onchange={() => queueSave()} />
      </section>

      <section class="card">
        <h2>Görünüm</h2>
        <p class="desc">Her grubun rengini ve sesini ayrı seç. Yazı boyutu ve simge şekli grubun önemine göre sabit kalır.</p>
        <div class="groups" role="tablist">
          {#each groups as g (g.id)}
            <button type="button" role="tab" aria-selected={g.id === styleGroup} class:on={g.id === styleGroup} onclick={() => (styleGroup = g.id)}>
              <span lang={g.id === 'divine' ? 'en' : undefined}>{g.label}</span>
              {#if customised(g)}<i class="custom-dot" title="Özelleştirildi"></i>{/if}
            </button>
          {/each}
        </div>
        {#if selGroup}
          <label class="field">
            <span>Renk</span>
            <select value={styleValue(selGroup)} onchange={(e) => setStyle(selGroup.id, e.currentTarget.value)}>
              {#if selGroup.allowDefault}<option value="default">{selGroup.defaultLabel}</option>{/if}
              {#each themes as t (t.id)}
                <option value={t.id}>{t.label}</option>
              {/each}
            </select>
          </label>
          <label class="field">
            <span>Ses</span>
            <span class="sound-row">
              <select value={cfg.sounds?.[selGroup.id] ?? ''} onchange={(e) => setSound(selGroup.id, e.currentTarget.value)}>
                <option value="">Varsayılan ({selGroup.defaultSound ? `oyun sesi ${selGroup.defaultSound}` : 'sessiz'})</option>
                <option value="none">Sessiz</option>
                {#each ['1', '2', '3', '4', '5', '6'] as n}
                  <option value={n}>Oyun sesi {n}</option>
                {/each}
                {#each sounds as f (f)}
                  <option value={'file:' + f}>{f}</option>
                {/each}
              </select>
              <button
                type="button"
                class="play"
                title={soundFile(selGroup) ? 'Dinle' : 'Oyun sesleri sadece oyunda çalınabilir'}
                aria-label="Sesi dinle"
                disabled={!soundFile(selGroup)}
                onclick={() => previewSound(selGroup)}
              >
                <svg viewBox="0 0 24 24"><path d="M8 5v14l11-7z" /></svg>
              </button>
            </span>
          </label>
          {#if soundError}<p class="error">{soundError}</p>{/if}
          <StylePreview group={selGroup} theme={paletteOf(selGroup)} sound={soundLabel(selGroup)} />
          {#if !sounds.length}
            <p class="desc hint">Kendi sesini kullanmak için bir mp3/wav dosyasını filtre klasörüne koy (Ayarlar'ın altındaki "Filtre klasörü").</p>
          {/if}
        {/if}
      </section>

      <section class="card">
        <h2>Otomatik güncelleme</h2>
        <Toggle bind:checked={cfg.auto_update_enabled} label="Açılışta ve düzenli aralıkla güncelle" onchange={() => queueSave(false)} />
        {#if cfg.auto_update_enabled}
          <Segmented
            small
            bind:value={cfg.auto_update_hours}
            onchange={() => queueSave(false)}
            options={[1, 2, 4, 6, 12].map((h) => ({ value: h, label: `${h} sa` }))}
          />
        {/if}
        <Toggle bind:checked={cfg.notify_enabled} label="Güncellenince bildirim göster" onchange={() => queueSave(false)} />
      </section>

      <section class="card">
        <h2>Trade taraması</h2>
        <p class="desc">Trade arama kotası (600 / 6 saat) trade sitesindeki kendi aramalarınla ortak.</p>
        <Segmented
          small
          bind:value={cfg.scan_budget_pct}
          onchange={() => queueSave(false)}
          options={[20, 40, 60].map((p) => ({ value: p, label: `Kotanın %${p}'ı` }))}
        />
      </section>

      <section class="card">
        <h2>Genel</h2>
        <label class="field">
          <span>Lig</span>
          <select bind:value={cfg.league_name} onchange={() => queueSave()}>
            {#each leagueOptions as l (l)}
              <option value={l}>{l}</option>
            {/each}
          </select>
        </label>
        {#if leagueUnlisted}
          <p class="desc hint">Bu lig güncel listede yok; seçimin korunuyor, istersen listeden yenisini seç.</p>
        {/if}
        <label class="field stack">
          <span>Oyundaki filtre adı</span>
          <input bind:value={cfg.filter_name} onchange={() => queueSave()} spellcheck="false" />
        </label>
        <label class="field stack">
          <span>Özel temel filtre (boşsa NeverSink)</span>
          <input bind:value={cfg.custom_base_filter} onchange={() => queueSave()} placeholder="C:\…\filtre.filter" spellcheck="false" />
        </label>
        <label class="field stack">
          <span>Fiyat sunucusu (ileride)</span>
          <input bind:value={cfg.price_source_url} onchange={() => queueSave(false)} placeholder="https://…/prices.json" spellcheck="false" />
        </label>
      </section>

      <section class="actions">
        <button onclick={() => AppService.OpenGameFolder()}>Filtre klasörü</button>
        <button onclick={() => AppService.OpenDataFolder()}>Veri klasörü</button>
        <button class="danger" onclick={() => AppService.Quit()}>Çıkış</button>
      </section>
      {#if meta}<p class="version">v{meta.version}{meta.testMode ? ' · test modu' : ''}</p>{/if}
    </div>
  {/if}
</main>

<style>
  main {
    height: 100%;
    display: flex;
    flex-direction: column;
    background:
      radial-gradient(120% 60% at 50% -10%, rgba(201, 164, 92, 0.1), transparent 60%),
      var(--bg);
    border: 1px solid var(--line-strong);
    border-radius: 12px;
    overflow: hidden;
  }

  header {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 12px 10px 10px 14px;
    border-bottom: 1px solid var(--line);
    --wails-draggable: drag;
  }
  header button {
    --wails-draggable: no-drag;
  }
  .emblem {
    width: 28px;
    height: 28px;
  }
  .brand {
    flex: 1;
    display: flex;
    align-items: baseline;
    gap: 8px;
    min-width: 0;
  }
  h1 {
    margin: 0;
    font-family: var(--serif);
    font-weight: 700;
    font-size: 16px;
    letter-spacing: 0.04em;
    color: var(--gold-bright);
  }
  .league {
    color: var(--muted);
    font-size: 11.5px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .save {
    font-size: 11px;
    color: var(--muted);
  }
  .save.saved {
    color: var(--ok);
  }
  .save.error {
    color: var(--bad);
  }
  .icon {
    width: 30px;
    height: 30px;
    display: grid;
    place-items: center;
    border: 0;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--text-2);
  }
  .icon:hover {
    background: var(--surface-2);
    color: var(--text);
  }
  .icon svg {
    width: 17px;
    height: 17px;
    fill: none;
    stroke: currentColor;
    stroke-width: 1.8;
    stroke-linecap: round;
    stroke-linejoin: round;
  }

  .scroll {
    flex: 1;
    overflow-y: auto;
    padding: 12px;
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  .card {
    background: linear-gradient(180deg, var(--surface-2), var(--surface));
    border: 1px solid var(--line);
    border-radius: var(--radius);
    padding: 12px 14px;
  }
  .card-title {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
  }
  h2 {
    margin: 0;
    font-family: var(--serif);
    font-weight: 500;
    font-size: 12.5px;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    color: var(--text-2);
  }
  .settings h2 {
    margin-bottom: 4px;
  }
  h3 {
    margin: 12px 0 6px;
    font-size: 12px;
    font-weight: 600;
    color: var(--text-2);
  }
  .groups {
    display: flex;
    flex-wrap: wrap;
    gap: 5px;
    margin-bottom: 4px;
  }
  .groups button {
    position: relative;
    padding: 5px 10px;
    border: 1px solid var(--line);
    border-radius: 999px;
    background: var(--bg);
    color: var(--text-2);
    font-size: 12px;
  }
  .groups button:hover {
    color: var(--text);
    border-color: var(--line-strong);
  }
  .groups button.on {
    border-color: var(--gold-dim);
    background: var(--surface-3);
    color: var(--gold-bright);
  }
  .custom-dot {
    display: inline-block;
    width: 6px;
    height: 6px;
    margin-left: 5px;
    vertical-align: middle;
    border-radius: 50%;
    background: var(--gold-bright);
  }
  .sound-row {
    display: flex;
    gap: 6px;
  }
  .play {
    width: 34px;
    display: grid;
    place-items: center;
    border: 1px solid var(--line);
    border-radius: var(--radius-sm);
    background: var(--bg);
    color: var(--gold-bright);
  }
  .play:disabled {
    color: var(--muted);
    cursor: default;
    opacity: 0.5;
  }
  .play svg {
    width: 14px;
    height: 14px;
    fill: currentColor;
  }
  .hint {
    margin-top: 8px;
  }
  .h3-note {
    margin-left: 4px;
    color: var(--muted);
    font-weight: 400;
    font-size: 11px;
  }
  .sub-title {
    margin-top: 16px;
    margin-bottom: 8px;
  }
  .aside {
    color: var(--muted);
    font-size: 12px;
  }
  .gold {
    color: var(--gold-bright);
    font-weight: 500;
  }
  .desc {
    margin: 4px 0 10px;
    color: var(--muted);
    font-size: 12px;
  }

  /* Status card */
  .status-head {
    display: flex;
    align-items: flex-start;
    gap: 10px;
  }
  .dot {
    flex: none;
    width: 10px;
    height: 10px;
    margin-top: 4px;
    border-radius: 50%;
    background: var(--muted);
  }
  .status.ok .dot {
    background: var(--ok);
    box-shadow: 0 0 0 4px rgba(124, 191, 107, 0.15);
  }
  .status.busy .dot {
    background: var(--gold-bright);
    animation: pulse 1.2s ease-in-out infinite;
  }
  .status.bad .dot {
    background: var(--bad);
  }
  .status-text {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }
  .status-text strong {
    font-size: 15px;
    font-weight: 600;
  }
  .sub {
    color: var(--text-2);
    font-size: 12px;
    overflow: hidden;
    text-overflow: ellipsis;
    display: -webkit-box;
    -webkit-line-clamp: 2;
    line-clamp: 2;
    -webkit-box-orient: vertical;
  }
  .status.bad .sub {
    color: #e39a95;
  }
  .notice {
    margin: 10px 0 0;
    padding: 7px 9px;
    border-radius: var(--radius-sm);
    background: rgba(224, 166, 74, 0.1);
    border: 1px solid rgba(224, 166, 74, 0.25);
    color: var(--warn);
    font-size: 12px;
  }
  .notice.bad {
    background: rgba(226, 92, 92, 0.1);
    border-color: rgba(226, 92, 92, 0.28);
    color: var(--bad);
  }
  .primary {
    width: 100%;
    margin-top: 12px;
    padding: 10px;
    border: 1px solid var(--gold);
    border-radius: var(--radius-sm);
    background: linear-gradient(180deg, #d9b56a, #a8813f);
    color: #1a140a;
    font-family: var(--serif);
    font-weight: 700;
    font-size: 13px;
    letter-spacing: 0.05em;
    transition: filter 0.12s;
  }
  .primary:hover:not(:disabled) {
    filter: brightness(1.08);
  }
  .primary:disabled {
    opacity: 0.55;
    cursor: default;
  }
  .primary.pulse:not(:disabled) {
    animation: glow 1.8s ease-in-out infinite;
  }
  .meta-row {
    display: flex;
    justify-content: space-between;
    gap: 8px;
    margin-top: 8px;
    color: var(--muted);
    font-size: 11.5px;
  }
  .meta-row em {
    font-style: normal;
    color: var(--text-2);
  }
  .reload {
    margin-left: auto;
  }
  .error {
    margin: 8px 0 0;
    color: var(--bad);
    font-size: 12px;
  }

  .bar {
    height: 6px;
    margin-top: 12px;
    border-radius: 6px;
    background: var(--bg);
    overflow: hidden;
  }
  .bar.thin {
    height: 4px;
    margin-top: 4px;
  }
  .bar span {
    display: block;
    height: 100%;
    background: linear-gradient(90deg, var(--gold-dim), var(--gold-bright));
    transition: width 0.3s ease;
  }
  .bar span.cyan {
    background: linear-gradient(90deg, #2f6f80, var(--exceptional));
  }

  /* Threshold */
  .threshold {
    display: flex;
    gap: 8px;
    align-items: stretch;
  }
  .threshold input {
    width: 96px;
    padding: 0 10px;
    background: var(--bg);
    border: 1px solid var(--line);
    border-radius: var(--radius-sm);
    font-size: 18px;
    font-weight: 600;
    color: var(--gold-bright);
    text-align: right;
    user-select: text;
  }
  .threshold input:focus {
    outline: none;
    border-color: var(--gold-dim);
  }
  .threshold :global(.seg) {
    flex: 1;
  }
  .chips {
    display: flex;
    gap: 6px;
    margin-top: 8px;
  }
  .chips button {
    flex: 1;
    padding: 4px 0;
    border: 1px solid var(--line);
    border-radius: 999px;
    background: transparent;
    color: var(--text-2);
    font-size: 12px;
  }
  .chips button:hover {
    border-color: var(--line-strong);
    color: var(--text);
  }
  .chips button.on {
    border-color: var(--gold-dim);
    color: var(--gold-bright);
  }

  /* Strictness slider */
  .strict {
    width: 100%;
    margin: 12px 0 2px;
    -webkit-appearance: none;
    appearance: none;
    height: 6px;
    border-radius: 6px;
    background: linear-gradient(90deg, var(--gold-dim) var(--p), var(--bg) var(--p));
    border: 1px solid var(--line);
  }
  .strict::-webkit-slider-thumb {
    -webkit-appearance: none;
    width: 16px;
    height: 16px;
    border-radius: 50%;
    background: var(--gold-bright);
    border: 3px solid var(--surface);
    box-shadow: 0 0 0 1px var(--gold-dim);
    cursor: pointer;
  }
  .scale {
    display: flex;
    justify-content: space-between;
    color: var(--muted);
    font-size: 11px;
  }

  /* Scan */
  .scan-line {
    display: flex;
    justify-content: space-between;
    gap: 8px;
    margin-top: 6px;
    font-size: 12px;
    color: var(--text-2);
  }
  .scan-line.muted {
    margin-top: 2px;
    color: var(--muted);
    font-size: 11.5px;
  }
  .ellipsis {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .scan-note {
    margin: 8px 0 0;
    color: var(--muted);
    font-size: 11px;
    line-height: 1.45;
  }
  .cyan-text {
    color: var(--exceptional);
  }

  /* Summary */
  .stats {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 8px;
  }
  .stats div {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 2px;
    padding: 10px 4px;
    background: var(--surface);
    border: 1px solid var(--line);
    border-radius: var(--radius);
  }
  .stats strong {
    font-size: 18px;
    font-weight: 600;
    color: var(--gold-bright);
  }
  .stats span {
    color: var(--muted);
    font-size: 11px;
  }
  .stats-caption {
    margin: -2px 0 0;
    text-align: center;
    color: var(--muted);
    font-size: 11px;
  }

  /* Settings */
  .field {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
    padding: 8px 0;
  }
  .field.stack {
    flex-direction: column;
    align-items: stretch;
    gap: 6px;
  }
  .field > span {
    font-weight: 500;
  }
  .field select,
  .field input {
    padding: 7px 9px;
    background: var(--bg);
    border: 1px solid var(--line);
    border-radius: var(--radius-sm);
    user-select: text;
  }
  .field select:focus,
  .field input:focus {
    outline: none;
    border-color: var(--gold-dim);
  }
  .actions {
    display: flex;
    gap: 8px;
  }
  .actions button {
    flex: 1;
    padding: 8px;
    border: 1px solid var(--line-strong);
    border-radius: var(--radius-sm);
    background: var(--surface-2);
    color: var(--text-2);
  }
  .actions button:hover {
    color: var(--text);
    border-color: var(--gold-dim);
  }
  .actions .danger:hover {
    color: var(--bad);
    border-color: var(--bad);
  }
  .version {
    margin: 0;
    text-align: center;
    color: var(--muted);
    font-size: 11px;
  }

  @keyframes pulse {
    50% {
      opacity: 0.35;
    }
  }
  @keyframes glow {
    50% {
      box-shadow: 0 0 0 4px rgba(236, 201, 124, 0.18);
    }
  }
</style>
