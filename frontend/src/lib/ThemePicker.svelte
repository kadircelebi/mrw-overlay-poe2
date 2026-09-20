<script lang="ts">
  import type { CustomStyle, StyleGroup, Theme } from '../../bindings/poe2filter/internal/filter/models'
  import Swatch from './Swatch.svelte'
  import Segmented from './Segmented.svelte'
  import { lookOf, toHex, colourName, shapeName, type Look } from './look'
  import { t } from './i18n.svelte'

  let {
    group,
    value,
    themes,
    nsThemes,
    custom,
    colours,
    shapes,
    current,
    onselect,
    oncustom,
  }: {
    group: StyleGroup
    value: string
    themes: Theme[]
    nsThemes: Theme[]
    custom: CustomStyle | undefined
    colours: string[]
    shapes: string[]
    current: Look
    onselect: (id: string) => void
    oncustom: (cs: CustomStyle) => void
  } = $props()

  type Tab = 'app' | 'ns' | 'custom'
  const tabOf = (v: string): Tab => (v === 'custom' ? 'custom' : v.startsWith('ns:') ? 'ns' : 'app')
  let tab = $state<Tab>('app')
  // Follow the group's saved choice when switching groups.
  $effect(() => {
    tab = tabOf(value)
  })

  const defaultTheme = $derived({ ...group.default, id: 'default', label: group.defaultLabel } as Theme)
  const appRows = $derived(group.allowDefault ? [defaultTheme, ...themes] : themes)

  // FilterBlade's order: the most important sections first, the rest as in the file.
  const sectionOrder = ['apex', 'currency', 'uniques', 'exotics', 'fragments', 'gear', 'typebased', 'maps']
  const nsSections = $derived.by(() => {
    const out: { category: string; rows: Theme[] }[] = []
    for (const t of nsThemes) {
      let s = out.find((x) => x.category === t.category)
      if (!s) out.push((s = { category: t.category ?? '', rows: [] }))
      s.rows.push(t)
    }
    const rank = (c: string) => (sectionOrder.includes(c) ? sectionOrder.indexOf(c) : sectionOrder.length)
    return out.sort((a, b) => rank(a.category) - rank(b.category))
  })

  // Editing a custom style starts from what the group looks like now.
  function seed(): CustomStyle {
    return {
      bg: toHex(current.bg, '#1e1e1e'),
      text: toHex(current.text, '#ffffff'),
      border: toHex(current.border, toHex(current.text, '#ffffff')),
      beam: colours.includes(current.beam.split(' ')[0]) ? current.beam.split(' ')[0] : '',
      icon: colours.includes(current.icon) ? current.icon : '',
      shape: shapes.includes(current.shape) ? current.shape : '',
    }
  }

  let draft = $state<CustomStyle>({ bg: '#1e1e1e', text: '#ffffff', border: '#ffffff', beam: '', icon: '', shape: '' })

  function openCustom() {
    draft = custom ? { ...custom } : seed()
    if (value !== 'custom') oncustom({ ...draft })
  }

  function update<K extends keyof CustomStyle>(k: K, v: CustomStyle[K]) {
    draft = { ...draft, [k]: v }
    if (k === 'shape' && v && !draft.icon) draft.icon = draft.beam || 'White'
    if (k === 'icon' && !v) draft.shape = ''
    oncustom({ ...draft })
  }

  $effect(() => {
    if (tab === 'custom' && custom) draft = { ...custom }
  })
</script>

<Segmented
  small
  value={tab}
  onchange={(v) => {
    tab = v
    if (v === 'custom') openCustom()
  }}
  options={[
    { value: 'app', label: t('picker.tabApp') },
    { value: 'ns', label: 'NeverSink' },
    { value: 'custom', label: t('picker.custom') },
  ]}
/>

{#if tab === 'app'}
  <div class="list" role="listbox" aria-label={t('picker.appThemes')}>
    {#each appRows as t (t.id)}
      <button type="button" role="option" aria-selected={value === t.id} class:on={value === t.id} onclick={() => onselect(t.id)}>
        <span class="name">{t.label}</span>
        <Swatch look={lookOf(group, t)} />
      </button>
    {/each}
  </div>
{:else if tab === 'ns'}
  {#if nsSections.length}
    <div class="list tall" role="listbox" aria-label={t('picker.nsThemes')}>
      {#each nsSections as s (s.category)}
        <div class="section">{s.category}</div>
        {#each s.rows as t (t.id)}
          <button type="button" role="option" aria-selected={value === t.id} class:on={value === t.id} onclick={() => onselect(t.id)}>
            <span class="name">{t.label} <em>({t.count}x)</em></span>
            <Swatch look={lookOf(group, t)} />
          </button>
        {/each}
      {/each}
    </div>
  {:else}
    <p class="empty">{t('picker.nsEmpty')}</p>
  {/if}
{:else}
  <div class="custom">
    <label><span>{t('picker.background')}</span><input type="color" value={draft.bg} oninput={(e) => update('bg', e.currentTarget.value)} /></label>
    <label><span>{t('picker.text')}</span><input type="color" value={draft.text} oninput={(e) => update('text', e.currentTarget.value)} /></label>
    <label><span>{t('picker.border')}</span><input type="color" value={draft.border} oninput={(e) => update('border', e.currentTarget.value)} /></label>
    <label class="wide">
      <span>{t('picker.beam')}</span>
      <select value={draft.beam} onchange={(e) => update('beam', e.currentTarget.value)}>
        <option value="">{t('picker.none')}</option>
        {#each colours as c}<option value={c}>{colourName(c) || c}</option>{/each}
      </select>
    </label>
    <label class="wide">
      <span>{t('picker.icon')}</span>
      <span class="pair">
        <select value={draft.icon} onchange={(e) => update('icon', e.currentTarget.value)}>
          <option value="">{t('picker.none')}</option>
          {#each colours as c}<option value={c}>{colourName(c) || c}</option>{/each}
        </select>
        <select value={draft.shape} disabled={!draft.icon} onchange={(e) => update('shape', e.currentTarget.value)}>
          <option value="">{t('picker.noShape')}</option>
          {#each shapes as s}<option value={s}>{shapeName(s) || s}</option>{/each}
        </select>
      </span>
    </label>
  </div>
{/if}

<style>
  .list {
    display: flex;
    flex-direction: column;
    gap: 3px;
    margin-top: 8px;
    max-height: 250px;
    overflow-y: auto;
    padding-right: 2px;
  }
  .list.tall {
    max-height: 320px;
  }
  .section {
    position: sticky;
    top: 0;
    z-index: 1;
    padding: 6px 4px 4px;
    background: var(--surface);
    color: var(--gold);
    font-family: var(--serif);
    font-size: 12px;
    letter-spacing: 0.05em;
  }
  .list button {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
    padding: 5px 8px;
    border: 1px solid transparent;
    border-radius: 6px;
    background: var(--bg);
    text-align: left;
  }
  .list button:hover {
    border-color: var(--line-strong);
  }
  .list button.on {
    border-color: var(--gold-dim);
    background: var(--surface-3);
  }
  .name {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 12px;
    color: var(--text-2);
  }
  .list button.on .name {
    color: var(--gold-bright);
  }
  .name em {
    font-style: normal;
    color: var(--muted);
  }
  .list :global(.swatch) {
    flex: none;
    width: 150px;
  }
  .empty {
    margin: 10px 0 0;
    color: var(--muted);
    font-size: 12px;
  }
  .custom {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 8px;
    margin-top: 10px;
  }
  .custom label {
    display: flex;
    flex-direction: column;
    gap: 4px;
    font-size: 11.5px;
    color: var(--text-2);
  }
  .custom label.wide {
    grid-column: 1 / -1;
    flex-direction: row;
    align-items: center;
    justify-content: space-between;
  }
  .pair {
    display: flex;
    gap: 6px;
  }
  .custom input[type='color'] {
    width: 100%;
    height: 30px;
    padding: 2px;
    border: 1px solid var(--line);
    border-radius: 6px;
    background: var(--bg);
    cursor: pointer;
  }
  .custom select {
    padding: 6px 8px;
    border: 1px solid var(--line);
    border-radius: 6px;
    background: var(--bg);
  }
</style>
