<script lang="ts">
  import { SearchItems } from '../../bindings/poe2filter/appservice'
  import type { SearchItem } from '../../bindings/poe2filter/internal/insights/models'

  let {
    items = $bindable([]),
    placeholder,
    onchange,
  }: { items: string[] | null; placeholder: string; onchange?: () => void } = $props()

  const list = $derived(items ?? [])

  let query = $state('')
  let results = $state<SearchItem[]>([])
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
        results = found.filter((r) => !list.includes(r.name))
        active = 0
      }
    }, 150)
  }

  function add(name: string) {
    if (!list.includes(name)) {
      items = [...list, name]
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
      add(results[active].name)
      e.preventDefault()
    }
  }
</script>

<div class="editor">
  {#if list.length}
    <div class="tags">
      {#each list as it (it)}
        <span class="tag">{it}<button type="button" aria-label="{it} kaldır" onclick={() => remove(it)}>×</button></span>
      {/each}
    </div>
  {/if}
  <div class="search">
    <input bind:value={query} oninput={search} onkeydown={key} {placeholder} spellcheck="false" />
    {#if results.length}
      <ul role="listbox">
        {#each results as r, i (r.name)}
          <li role="option" aria-selected={i === active}>
            <button type="button" class:active={i === active} onmouseenter={() => (active = i)} onclick={() => add(r.name)}>
              <span class="name">{r.name}</span>
              <span class="cat">{r.category}</span>
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
    border-radius: 999px;
    font-size: 12px;
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
    border-radius: 5px;
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
  .cat {
    flex: none;
    color: var(--muted);
    font-size: 11px;
  }
</style>
