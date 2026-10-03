<script lang="ts">
  import { AppService } from '../../bindings/poe2filter'
  import type { ExoticCandidate } from '../../bindings/poe2filter/models'
  import { t } from './i18n.svelte'

  // Alt+E: put the item's base or one of its named modifiers into the Exotic
  // group with a click.
  let { candidates }: { candidates: ExoticCandidate[] } = $props()

  let open = $state(false)
  let busy = $state('')
  let added = $state<Record<string, boolean>>({})
  let note = $state('')

  function keyOf(c: ExoticCandidate): string {
    return c.entry.kind === 'base' ? `b|${c.entry.base}` : `m|${(c.entry.names ?? []).join(',')}`
  }

  async function add(c: ExoticCandidate) {
    const k = keyOf(c)
    if (busy || c.present || added[k]) return
    busy = k
    try {
      const res = await AppService.AddExotic(c.entry)
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
        {@const done = c.present || added[keyOf(c)]}
        <button type="button" class:done disabled={done || !!busy} onclick={() => add(c)} title={done ? t('exoticAdd.present') : t('exoticAdd.add')}>
          {done ? '✓' : '+'} {c.entry.kind === 'base' ? t('exoticAdd.base', c.entry.base ?? '') : c.entry.label}
        </button>
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
  .chips button.done { border-color: #2a3a33; color: #6f8a7e; }
  .note { margin: 0; padding: 0 10px 8px; color: #8fd18a; font-size: 10.5px; }
</style>
