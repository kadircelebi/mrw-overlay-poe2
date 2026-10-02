<script lang="ts">
  import { t } from './i18n.svelte'
  import { SearchItems } from '../../bindings/poe2filter/appservice'
  import type { SearchItem } from '../../bindings/poe2filter/internal/insights/models'

  let {
    items = $bindable([]),
    placeholder,
    onchange,
    uniqueVariants = false,
    stacks = false,
  }: {
    items: string[] | null
    placeholder: string
    onchange?: () => void
    /** Offer "unique only" entries for bases that have uniques. */
    uniqueVariants?: boolean
    /** Let each entry ask for a minimum stack size ("Simulacrum Splinter|x15"). */
    stacks?: boolean
  } = $props()

  const UNIQUE = '|unique'
  const STACK = /\|x(\d+)$/
  type Option = { value: string; name: string; note: string; unique: boolean; members?: string[] }

  const list = $derived(items ?? [])

  let query = $state('')
  let results = $state<Option[]>([])

  function price(r: SearchItem): string {
    if (r.price_divine && r.price_divine >= 1) return `${r.price_divine >= 10 ? r.price_divine.toFixed(0) : r.price_divine.toFixed(1)} div`
    if (r.price_exalt) return `${r.price_exalt.toFixed(0)} ex`
    return ''
  }

  function toOptions(found: SearchItem[]): Option[] {
    const out: Option[] = []
    for (const r of found) {
      const p = price(r)
      if (r.type === 'family') {
        // Members already in the list (with any stack size) are not added twice.
        const members = (r.members ?? []).filter((m) => !list.some((i) => label(i).name === m && !label(i).unique))
        if (members.length) {
          out.push({ value: 'family:' + r.name, name: t('editor.familyAll', r.name), unique: false,
            note: t('editor.familyCount', members.length), members })
        }
      } else if (uniqueVariants && r.type === 'base' && r.related_uniques?.length) {
        out.push({ value: r.name + UNIQUE, name: r.name, unique: true,
          note: p ? t('editor.uniqueOnlyTop', p) : t('editor.uniqueOnly') })
        out.push({ value: r.name, name: r.name, unique: false, note: t('editor.allRarities') })
      } else {
        out.push({ value: r.name, name: r.name, unique: false, note: p ? `${r.category} · ${p}` : r.category })
      }
    }
    return out
  }

  function label(v: string): { name: string; unique: boolean; stack: number } {
    if (v.endsWith(UNIQUE)) return { name: v.slice(0, -UNIQUE.length), unique: true, stack: 0 }
    const m = STACK.exec(v)
    return m ? { name: v.slice(0, m.index), unique: false, stack: Number(m[1]) } : { name: v, unique: false, stack: 0 }
  }

  // A stack size is part of the entry: "Verisium|x500". An empty box means
  // every stack. The same item may stay in the list with another size.
  function setStack(entry: string, value: string) {
    const l = label(entry)
    const n = Math.max(0, Math.min(5000, Math.floor(Number(value) || 0)))
    const next = n > 0 ? `${l.name}|x${n}` : l.name
    if (next === entry) return
    if (list.includes(next)) {
      items = list.filter((i) => i !== entry)
    } else {
      items = list.map((i) => (i === entry ? next : i))
    }
    onchange?.()
  }
  let active = $state(0)
  let timer: ReturnType<typeof setTimeout> | undefined

  function search() {
    clearTimeout(timer)
    const q = query.trim()
    if (q.length < 2) {
      results = []
      return
    }
    timer = setTimeout(async () => {
      const found = (await SearchItems(q)) ?? []
      if (query.trim() === q) {
        results = toOptions(found).filter((o) => !list.includes(o.value))
        active = 0
      }
    }, 150)
  }

  function add(o: Option) {
    const names = (o.members ?? [o.value]).filter((n) => !list.includes(n))
    if (names.length) {
      items = [...list, ...names]
      onchange?.()
    }
    query = ''
    results = []
  }

  function remove(name: string) {
    items = list.filter((i) => i !== name)
    onchange?.()
  }

  function key(e: KeyboardEvent) {
    if (!results.length) return
    if (e.key === 'ArrowDown') {
      active = (active + 1) % results.length
      e.preventDefault()
    } else if (e.key === 'ArrowUp') {
      active = (active - 1 + results.length) % results.length
      e.preventDefault()
    } else if (e.key === 'Enter') {
      add(results[active])
      e.preventDefault()
    }
  }
</script>

<div class="editor">
  {#if list.length}
    <div class="tags">
      {#each list as it (it)}
        {@const l = label(it)}
        <span class="tag" class:stacked={l.stack > 0}>{l.name}{#if l.unique}<em class="u">Unique</em>{/if}{#if stacks && !l.unique}<label class="stack" title={t('editor.stackHint')}>×<input
                type="number"
                min="0"
                max="5000"
                step="1"
                value={l.stack || ''}
                placeholder={t('editor.stackAny')}
                aria-label={t('editor.stackLabel', l.name)}
                onchange={(e) => setStack(it, e.currentTarget.value)}
                onkeydown={(e) => {
                  if (e.key === 'Enter') e.currentTarget.blur()
                }}
              /></label>{/if}<button type="button" aria-label={t('editor.remove', l.name)} onclick={() => remove(it)}>×</button></span>
      {/each}
    </div>
  {/if}
  {#if stacks && list.length}
    <p class="stack-hint">{t('editor.stackHint')}</p>
  {/if}
  <div class="search">
    <input bind:value={query} oninput={search} onkeydown={key} {placeholder} spellcheck="false" />
    {#if results.length}
      <ul role="listbox">
        {#each results as r, i (r.value)}
          <li role="option" aria-selected={i === active}>
            <button type="button" class:active={i === active} onmouseenter={() => (active = i)} onclick={() => add(r)}>
              <span class="name" class:unique={r.unique} class:family={!!r.members}>{r.name}</span>
              <span class="cat">{r.note}</span>
            </button>
          </li>
        {/each}
      </ul>
    {/if}
  </div>
</div>

<style>
  .editor {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .tags {
    display: flex;
    flex-wrap: wrap;
    gap: 5px;
  }
  .tag {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 3px 4px 3px 9px;
    background: var(--surface-3);
    border: 1px solid var(--line-strong);
    border-radius: var(--radius-sm);
    font-size: 12px;
  }
  .tag.stacked {
    border-color: var(--gold-dim);
  }
  .stack {
    display: inline-flex;
    align-items: center;
    gap: 1px;
    margin-left: 4px;
    color: var(--muted);
    font-size: 11px;
  }
  .stack input {
    width: 46px;
    padding: 1px 4px;
    font-size: 11.5px;
    background: var(--bg);
    border: 1px solid var(--line);
    border-radius: var(--radius-sm);
  }
  .stack input::placeholder {
    color: var(--muted);
  }
  .stack-hint {
    margin: 0;
    color: var(--muted);
    font-size: 11.5px;
  }
  .tag button {
    width: 18px;
    height: 18px;
    border: 0;
    border-radius: 50%;
    background: transparent;
    color: var(--muted);
    line-height: 1;
  }
  .tag button:hover {
    background: var(--line-strong);
    color: var(--text);
  }
  .search {
    position: relative;
  }
  input {
    width: 100%;
    padding: 8px 10px;
    background: var(--bg);
    border: 1px solid var(--line);
    border-radius: var(--radius-sm);
    user-select: text;
  }
  input:focus {
    border-color: var(--gold-dim);
    outline: none;
  }
  ul {
    position: absolute;
    z-index: 5;
    left: 0;
    right: 0;
    top: calc(100% + 4px);
    margin: 0;
    padding: 4px;
    list-style: none;
    background: var(--surface-2);
    border: 1px solid var(--line-strong);
    border-radius: var(--radius-sm);
    box-shadow: 0 10px 30px rgba(0, 0, 0, 0.5);
  }
  li button {
    display: flex;
    justify-content: space-between;
    gap: 8px;
    width: 100%;
    padding: 6px 8px;
    border: 0;
    border-radius: var(--radius-sm);
    background: transparent;
    text-align: left;
  }
  li button.active {
    background: var(--surface-3);
  }
  .name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .name.unique {
    color: #e6893a;
  }
  .name.family {
    color: var(--gold);
    font-weight: 600;
  }
  .u {
    margin-left: 5px;
    padding: 0 5px;
    border-radius: var(--radius-sm);
    background: rgba(230, 137, 58, 0.18);
    color: #f0a766;
    font-style: normal;
    font-size: 10.5px;
    font-weight: 600;
  }
  .cat {
    flex: none;
    color: var(--muted);
    font-size: 11px;
  }
</style>
