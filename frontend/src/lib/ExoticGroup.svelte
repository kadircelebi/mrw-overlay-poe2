<script lang="ts">
  import { onMount } from 'svelte'
  import { AppService } from '../../bindings/poe2filter/internal/app'
  import type { Config, ExoticEntry } from '../../bindings/poe2filter/internal/filter/models'
  import type { ExoticModOption } from '../../bindings/poe2filter/internal/app/models'
  import Segmented from './Segmented.svelte'
  import Toggle from './Toggle.svelte'
  import { modLabel, range, tierOption, tiersUpTo, userKey } from './exoticTiers'
  import { t } from './i18n.svelte'

  // The Exotic group: NeverSink's exotic bases and modifiers with the
  // player's changes laid over them (cfg.exotic keeps only the changes).
  let { cfg, onchange }: { cfg: Config; onchange: () => void } = $props()

  // Item classes whose explicit modifiers the filter can look at.
  const classes = [
    'Amulets', 'Rings', 'Belts', 'Body Armours', 'Boots', 'Gloves', 'Helmets', 'Shields', 'Bucklers', 'Foci', 'Quivers',
    'Bows', 'Crossbows', 'One Hand Maces', 'Two Hand Maces', 'Quarterstaves', 'Spears', 'Sceptres', 'Staves', 'Talismans', 'Wands', 'Jewels',
  ]

  type Row = ExoticEntry & { off: boolean; changed: boolean }

  let ns = $state<ExoticEntry[]>([])
  let pick = $state('bases')
  let newBase = $state('')
  let confirmReset = $state(false)

  onMount(() => {
    AppService.ExoticNeverSink().then((list) => (ns = list ?? [])).catch(() => {})
  })

  function settings() {
    cfg.exotic ??= { added: [], removed: [], levels: {} }
    cfg.exotic.added ??= []
    cfg.exotic.removed ??= []
    cfg.exotic.levels ??= {}
    return cfg.exotic
  }

  // Same as filter.ExoticRows: NeverSink's entries, then the player's.
  const rows = $derived.by((): Row[] => {
    const x = cfg.exotic
    const off = new Set(x?.removed ?? [])
    const levels = x?.levels ?? {}
    return [...ns, ...(x?.added ?? [])].map((e) => {
      const lv = levels[e.key]
      const moved = !!lv && lv !== e.level
      return { ...e, level: moved ? lv! : e.level, off: off.has(e.key), changed: moved && e.source === 'neversink' }
    })
  })
  const baseRows = $derived(rows.filter((r) => r.kind === 'base'))
  const modRows = $derived(rows.filter((r) => r.kind === 'mod' && (r.classes ?? []).includes(pick)))
  function modCount(cls: string): number {
    return rows.filter((r) => r.kind === 'mod' && !r.off && (r.classes ?? []).includes(cls)).length
  }
  const changes = $derived((cfg.exotic?.added?.length ?? 0) + (cfg.exotic?.removed?.length ?? 0) + Object.keys(cfg.exotic?.levels ?? {}).length)

  function setLevel(r: Row, level: string) {
    const x = settings()
    if (r.source === 'user') {
      const e = x.added!.find((a) => a.key === r.key)
      if (e) e.level = level
    } else {
      const nsLevel = ns.find((e) => e.key === r.key)?.level
      if (level === nsLevel) delete x.levels![r.key]
      else x.levels![r.key] = level
    }
    onchange()
  }

  function toggle(r: Row) {
    const x = settings()
    if (r.source === 'user') {
      x.added = x.added!.filter((a) => a.key !== r.key)
      delete x.levels![r.key]
    } else if (r.off) {
      x.removed = x.removed!.filter((k) => k !== r.key)
    } else {
      x.removed = [...x.removed!, r.key]
    }
    onchange()
  }

  function addBase() {
    const base = newBase.trim()
    if (!base) return
    const x = settings()
    x.added = [...x.added!, { key: '', kind: 'base', base, level: 'normal', source: 'user' }]
    newBase = ''
    onchange()
  }

  function reset() {
    if (!confirmReset) {
      confirmReset = true
      return
    }
    cfg.exotic = { added: [], removed: [], levels: {} }
    confirmReset = false
    onchange()
  }

  // ---- adding a modifier to the picked class ----
  const optionCache: Record<string, ExoticModOption[]> = {}
  let options = $state<ExoticModOption[]>([])
  let query = $state('')
  let chosen = $state<ExoticModOption | null>(null)
  let minTier = $state(1)
  let newLevel = $state('normal')

  $effect(() => {
    const cls = pick
    chosen = null
    query = ''
    if (cls === 'bases') return
    if (optionCache[cls]) {
      options = optionCache[cls]
      return
    }
    options = []
    AppService.ExoticModOptions(cls).then((list) => {
      optionCache[cls] = list ?? []
      if (pick === cls) options = optionCache[cls]
    }).catch(() => {})
  })

  const matches = $derived.by(() => {
    const q = query.trim().toLowerCase()
    if (q.length < 2) return []
    return options.filter((o) => o.text.toLowerCase().includes(q)).slice(0, 30)
  })
  const kept = $derived(chosen ? tiersUpTo(chosen.tiers, minTier) : [])
  const clashes = $derived([...new Set(kept.flatMap((x) => (x.also ?? []).map((a) => `${x.name} → ${a}`)))])

  function choose(o: ExoticModOption) {
    chosen = o
    minTier = o.tiers?.[0]?.tier ?? 1
    newLevel = 'normal'
  }

  function addMod() {
    if (!chosen || !kept.length) return
    const x = settings()
    x.added = [...x.added!, {
      key: '', kind: 'mod', classes: [pick], names: kept.map((k) => k.name), stat: chosen.stat,
      min_tier: minTier, label: modLabel(chosen.text, kept), level: newLevel, source: 'user',
    }]
    chosen = null
    query = ''
    onchange()
  }

  // ---- changing the tiers of the player's modifier entry ----
  // The option an entry was made from: its stat, or (added from Alt+E
  // before entries kept the stat) a tier name it uses.
  function optionOf(r: Row): ExoticModOption | undefined {
    const names = new Set((r.names ?? []).map((n) => n.toLowerCase()))
    const uses = (o: ExoticModOption) => (o.tiers ?? []).some((x) => names.has(x.name.toLowerCase()))
    return options.find((o) => o.stat === r.stat && uses(o)) ?? options.find(uses)
  }

  // tierOf is the lowest tier the entry keeps.
  function tierOf(r: Row, o: ExoticModOption): number {
    if (r.min_tier) return r.min_tier
    const names = new Set((r.names ?? []).map((n) => n.toLowerCase()))
    return Math.max(...(o.tiers ?? []).filter((x) => names.has(x.name.toLowerCase())).map((x) => x.tier))
  }

  function setTier(r: Row, o: ExoticModOption, tier: number) {
    const e = settings().added!.find((a) => a.key === r.key)
    if (!e) return
    const keep = tiersUpTo(o.tiers, tier)
    e.names = keep.map((k) => k.name)
    e.stat = o.stat
    e.min_tier = tier
    e.label = modLabel(o.text, keep)
    e.key = userKey(e)
    onchange()
  }

  function rowName(r: Row): string {
    if (r.kind === 'base') return r.base ?? ''
    if (r.label) return r.label
    const others = (r.classes ?? []).filter((c) => c !== pick)
    return `${(r.names ?? []).join(', ')}${others.length ? ` (${others.join(', ')})` : ''}`
  }
</script>

<div class="exotic-group">
  <section class="card">
    <Toggle bind:checked={cfg.show_exotics} label={t('exotic.on')} hint={t('exotic.hint')} onchange={onchange} />
    <div class="reset-row">
      <span class="desc hint">{changes ? t('exotic.changes', changes) : t('exotic.pristine')}</span>
      <button type="button" class="ghost" class:confirm={confirmReset} disabled={!changes} onclick={reset} onblur={() => (confirmReset = false)}>
        {confirmReset ? t('exotic.resetConfirm') : t('exotic.reset')}
      </button>
    </div>
  </section>

  <div class="split">
    <div class="kinds" role="list">
      <button type="button" class:on={pick === 'bases'} onclick={() => (pick = 'bases')}>
        <span>{t('exotic.bases')}</span><small class="num">{baseRows.filter((r) => !r.off).length}</small>
      </button>
      {#each classes as cls (cls)}
        <button type="button" class:on={pick === cls} onclick={() => (pick = cls)}>
          <span>{cls}</span><small class="num">{modCount(cls) || ''}</small>
        </button>
      {/each}
    </div>

    <div class="entries">
      {#if pick === 'bases'}
        <p class="desc hint">{t('exotic.basesHint')}</p>
        {#each baseRows as r (r.key)}
          {@render row(r)}
        {/each}
        <div class="add">
          <input bind:value={newBase} placeholder={t('exotic.basePlaceholder')} onkeydown={(e) => e.key === 'Enter' && addBase()} />
          <button type="button" onclick={addBase} disabled={!newBase.trim()}>{t('exotic.add')}</button>
        </div>
      {:else}
        <p class="desc hint">{t('exotic.modsHint')}</p>
        {#each modRows as r (r.key)}
          {@render row(r)}
        {:else}
          <p class="desc hint">{t('exotic.noMods', pick)}</p>
        {/each}
        <div class="add-mod">
          <input bind:value={query} placeholder={t('exotic.modSearch')} oninput={() => (chosen = null)} />
          {#if !chosen && matches.length}
            <div class="matches">
              {#each matches as o (o.stat + o.text)}
                <button type="button" onclick={() => choose(o)}>
                  <span>{o.text}</span><small>{o.affix}{#if o.pool} · <b class="pool">{t('exotic.pool')}</b>{/if} · T1 {o.tiers?.[0] ? range(o.tiers[0]) : ''}</small>
                </button>
              {/each}
            </div>
          {:else if !chosen && query.trim().length >= 2}
            <p class="desc hint">{options.length ? t('exotic.noMatch') : t('exotic.loading')}</p>
          {/if}
          {#if chosen}
            <div class="chosen">
              <strong>{chosen.text}</strong>
              {#if chosen.pool}<p class="desc hint">{t('exotic.poolHint')}</p>{/if}
              <label class="field stack">
                <span>{t('exotic.minTier')}</span>
                <select bind:value={minTier}>
                  {#each chosen.tiers ?? [] as x (x.tier)}
                    <option value={x.tier}>{tierOption(x)}</option>
                  {/each}
                </select>
              </label>
              <p class="desc hint">{t('exotic.names', kept.map((k) => k.name).join(', '))}</p>
              {#if clashes.length}<p class="desc warn">{t('exotic.clash', clashes.join('; '))}</p>{/if}
              <Segmented small bind:value={newLevel} options={[{ value: 'high', label: t('exotic.high') }, { value: 'normal', label: t('exotic.normal') }]} />
              <div class="add">
                <button type="button" onclick={addMod}>{t('exotic.addMod')}</button>
                <button type="button" class="ghost" onclick={() => (chosen = null)}>{t('hide.cancel')}</button>
              </div>
            </div>
          {/if}
        </div>
      {/if}
    </div>
  </div>
</div>

{#snippet row(r: Row)}
  <div class="row" class:off={r.off}>
    <span class="name" title={[...(r.names ?? []), ...(r.conds ?? [])].join(' · ')}>{rowName(r)}</span>
    <span class="badge" class:user={r.source === 'user'} class:offb={r.off}>
      {r.off ? t('exotic.srcOff') : r.source === 'user' ? t('exotic.srcUser') : 'NeverSink'}
    </span>
    {#if r.source === 'user' && r.kind === 'mod'}
      {@const o = optionOf(r)}
      {#if o}
        <select class="tier" value={tierOf(r, o)} title={t('exotic.names', (r.names ?? []).join(', '))} onchange={(e) => setTier(r, o, Number(e.currentTarget.value))}>
          {#each o.tiers ?? [] as x (x.tier)}
            <option value={x.tier}>{t('exotic.tierUp', `T${x.tier}`)} · {range(x)} · "{x.name}"</option>
          {/each}
        </select>
      {/if}
    {/if}
    {#if !r.off}
      <Segmented small value={r.level} onchange={(v) => setLevel(r, v)} options={[{ value: 'high', label: t('exotic.high') }, { value: 'normal', label: t('exotic.normal') }]} />
    {/if}
    <button type="button" class="ghost" onclick={() => toggle(r)}>
      {r.source === 'user' ? t('hidden.remove') : r.off ? t('exotic.turnOn') : t('exotic.turnOff')}
    </button>
  </div>
{/snippet}

<style>
  .exotic-group { display: grid; gap: 10px; }
  .reset-row { display: flex; align-items: center; justify-content: space-between; gap: 10px; margin-top: 8px; }
  .reset-row .confirm { color: var(--bad); border-color: var(--bad); }
  .split { display: grid; grid-template-columns: 170px 1fr; gap: 10px; min-height: 0; }
  .kinds { display: flex; flex-direction: column; max-height: 420px; overflow: auto; border: 1px solid var(--line); }
  .kinds button { display: flex; justify-content: space-between; gap: 6px; padding: 6px 10px; border: 0; border-bottom: 1px solid var(--line); border-left: 2px solid transparent; background: none; color: var(--text-2); text-align: left; font-size: 12px; }
  .kinds button.on { color: var(--gold-bright); border-left-color: var(--gold); background: linear-gradient(90deg, var(--ui-tint,rgba(194, 174, 126, 0.13)), transparent); }
  .kinds small { color: var(--muted); }
  .entries { min-width: 0; }
  .row { display: flex; align-items: center; gap: 8px; padding: 6px 0; border-top: 1px solid var(--line); }
  .row.off .name { color: var(--muted); text-decoration: line-through; }
  .name { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .badge { flex: none; padding: 1px 6px; border: 1px solid var(--line-strong); border-radius: var(--radius-sm); color: var(--text-2); font-size: 10.5px; }
  .badge.user { color: var(--gold); border-color: var(--gold-dim); }
  .badge.offb { color: var(--muted); }
  .add { display: flex; gap: 6px; margin-top: 8px; }
  .add input, .add-mod input, .chosen select { flex: 1; min-width: 0; padding: 6px 8px; border: 1px solid var(--line-strong); background: var(--bg-2, var(--ui-surface,#191b18)); color: var(--text); }
  .add-mod { margin-top: 10px; display: grid; gap: 6px; }
  .matches { display: flex; flex-direction: column; max-height: 220px; overflow: auto; border: 1px solid var(--line); }
  .matches button { display: flex; flex-direction: column; align-items: flex-start; gap: 2px; padding: 6px 8px; border: 0; border-bottom: 1px solid var(--line); background: none; color: var(--text-2); text-align: left; }
  .matches button:hover { background: var(--ui-tint,rgba(255, 255, 255, 0.04)); }
  .matches small { color: var(--muted); font-size: 10.5px; }
  .matches .pool { color: var(--ui-text,#b48cf0); font-weight: normal; }
  .chosen { display: grid; gap: 6px; padding: 8px; border: 1px solid var(--line-strong); }
  .row:has(.tier) { flex-wrap: wrap; }
  .row:has(.tier) .name { flex-basis: 100%; }
  .row:has(.tier) .badge { margin-left: auto; }
  .row .tier { flex: 0 1 auto; min-width: 0; max-width: 260px; padding: 3px 6px; border: 1px solid var(--line-strong); background: var(--bg-2, var(--ui-surface,#191b18)); color: var(--text-2); font-size: 11px; }
  .chosen strong { color: var(--gold-bright); font-weight: normal; }
</style>
