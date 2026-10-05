<script lang="ts">
  import { AppService } from '../../bindings/poe2filter'
  import type { ExoticCandidate } from '../../bindings/poe2filter/models'
  import { modLabel, range, tiersUpTo } from './exoticTiers'
  import { t } from './i18n.svelte'

  // Alt+E: put the item's base or one of its named modifiers into the Exotic
  // group with a click.
  let { candidates }: { candidates: ExoticCandidate[] } = $props()

  let open = $state(false)
  let busy = $state('')
  let added = $state<Record<string, boolean>>({})
  let note = $state('')
  // The lowest tier picked per modifier (the item's own tier at first).
  let picked = $state<Record<string, number>>({})

  // keyOf tells candidates apart by the item's own modifier name: a prefix
  // and a suffix can share a stat (item rarity), never a name.
  function keyOf(c: ExoticCandidate): string {
    if (c.entry.kind === 'base') return `b|${c.entry.base}`
    const own = c.tiers?.find((x) => x.tier === c.tier)?.name
    return `m|${c.entry.stat ?? ''}|${own ?? (c.entry.names ?? []).join(',')}`
  }

  function tierOf(c: ExoticCandidate): number {
    return picked[keyOf(c)] ?? c.tier ?? 0
  }

  // entryOf is the entry with the picked tiers: that tier and every better one.
  function entryOf(c: ExoticCandidate) {
    if (!c.tiers?.length) return c.entry
    const keep = tiersUpTo(c.tiers, tierOf(c))
    return { ...c.entry, names: keep.map((x) => x.name), min_tier: tierOf(c), label: modLabel(c.text ?? '', keep) }
  }

  // doneKey changes with the picked tier, so a wider pick can be added again.
  function doneKey(c: ExoticCandidate): string {
    return `${keyOf(c)}|${tierOf(c)}`
  }

  function isDone(c: ExoticCandidate): boolean {
    if (added[doneKey(c)]) return true
    if (!c.tiers?.length) return c.present
    const have = new Set((c.covered ?? []).map((n) => n.toLowerCase()))
    return (entryOf(c).names ?? []).every((n) => have.has(n.toLowerCase()))
  }

  async function add(c: ExoticCandidate) {
    const k = doneKey(c)
    if (busy || isDone(c)) return
    busy = k
    try {
      const res = await AppService.AddExotic(entryOf(c))
      added = { ...added, [k]: true }
      note = t(res.updating ? 'exoticAdd.done' : 'exoticAdd.doneLater')
    } catch (err) {
      note = String(err).replace(/^RuntimeError:\s*/i, '')
    } finally {
      busy = ''
    }
  }
</script>

<section class="exotic-add">
  <button type="button" class="line" onclick={() => (open = !open)}>
    <b>{t('exoticAdd.title')}</b><span>{t('exoticAdd.hint')}</span><em>{open ? '▴' : '▾'}</em>
  </button>
  {#if open}
    <div class="chips">
      {#each candidates as c (keyOf(c))}
        {@const done = isDone(c)}
        <span class="pick">
          <button type="button" class:done disabled={done || !!busy} onclick={() => add(c)} title={done ? t('exoticAdd.present') : t('exoticAdd.add')}>
            {done ? '✓' : '+'} {c.entry.kind === 'base' ? t('exoticAdd.base', c.entry.base ?? '') : entryOf(c).label}
          </button>
          {#if c.tiers?.length}
            <select value={tierOf(c)} title={t('exotic.minTier')} onchange={(e) => (picked = { ...picked, [keyOf(c)]: Number(e.currentTarget.value) })}>
              {#each c.tiers as x (x.tier)}
                <option value={x.tier}>{t('exotic.tierUp', `T${x.tier}`)} · {range(x)}</option>
              {/each}
            </select>
          {/if}
        </span>
      {/each}
    </div>
    {#if note}<p class="note">{note}</p>{/if}
  {/if}
</section>

<style>
  .exotic-add { margin-top: 6px; border: 1px solid #2e3a34; background: #0f1311; }
  .line { display: flex; align-items: center; gap: 8px; width: 100%; padding: 6px 10px; border: 0; background: none; color: var(--muted); text-align: left; font-size: 11px; }
  .line:hover { background: #151a17; }
  .line b { color: #6fe0b8; font-weight: 600; }
  .line span { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .line em { font-style: normal; color: #6f746c; }
  .chips { display: flex; flex-wrap: wrap; gap: 5px; padding: 4px 10px 8px; }
  .chips button { padding: 3px 8px; border: 1px solid #2f5a4a; border-radius: 2px; background: #121a16; color: #b8e0cf; font-size: 10.5px; text-align: left; }
  .chips button:hover:not(:disabled) { background: #183026; }
  .pick { display: inline-flex; }
  .pick select { margin-left: -1px; padding: 2px 4px; border: 1px solid #2f5a4a; border-radius: 2px; background: #121a16; color: #8fbfaa; font-size: 10.5px; }
  .chips button.done { border-color: #2a3a33; color: #6f8a7e; }
  .note { margin: 0; padding: 0 10px 8px; color: #8fd18a; font-size: 10.5px; }
</style>
