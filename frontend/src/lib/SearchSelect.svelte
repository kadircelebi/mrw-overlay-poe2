<script lang="ts">
  // A select you can type into: the field shows the chosen option, focusing it
  // opens the whole list, and typing narrows it the way the affix search does
  // (wordMatcher: every typed word must appear).
  import { wordMatcher } from './wordMatch'
  type Option = { id?: string | null; text: string }
  let { options, value, onchange, label = '' }: {
    options: Option[]
    value: string
    onchange: (value: string) => void
    label?: string
  } = $props()

  const listId = $props.id()
  let open = $state(false)
  let query = $state('')
  let active = $state(0)
  let list = $state<HTMLDivElement | null>(null)

  const selectedText = $derived(options.find((option) => (option.id ?? '') === value)?.text ?? '')
  const shown = $derived.by(() => {
    if (!query.trim()) return options
    const matches = wordMatcher(query)
    return options.filter((option) => matches(option.text))
  })

  function openList() {
    query = ''
    open = true
    active = Math.max(0, options.findIndex((option) => (option.id ?? '') === value))
    queueMicrotask(scrollActive)
  }
  function close() { open = false; query = '' }
  function pick(option: Option) {
    onchange(option.id ?? '')
    close()
  }
  function scrollActive() {
    list?.children[active]?.scrollIntoView({ block: 'nearest' })
  }
  function keydown(event: KeyboardEvent) {
    if (!open && (event.key === 'ArrowDown' || event.key === 'ArrowUp')) { openList(); event.preventDefault(); return }
    if (!open) return
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault()
      if (!shown.length) return
      active = (active + (event.key === 'ArrowDown' ? 1 : -1) + shown.length) % shown.length
      queueMicrotask(scrollActive)
    } else if (event.key === 'Enter') {
      event.preventDefault()
      if (shown[active]) pick(shown[active])
    } else if (event.key === 'Escape') {
      event.preventDefault()
      close()
      ;(event.currentTarget as HTMLInputElement).blur()
    }
  }
</script>

<span class="search-select">
  <input
    value={open ? query : selectedText}
    placeholder={open ? selectedText : ''}
    aria-label={label || undefined}
    role="combobox"
    aria-expanded={open}
    aria-controls={listId}
    spellcheck="false"
    onfocus={openList}
    onclick={() => { if (!open) openList() }}
    onblur={close}
    oninput={(event) => { query = event.currentTarget.value; active = 0; open = true }}
    onkeydown={keydown}
  />
  <span class="caret" aria-hidden="true">▾</span>
  {#if open && shown.length}
    <!-- mousedown keeps the focus in the field, so a click on the list or its
         scrollbar does not close it. -->
    <div class="options" id={listId} role="listbox" tabindex="-1" bind:this={list} onmousedown={(event) => event.preventDefault()}>
      {#each shown as option, index}
        <button type="button" role="option" aria-selected={(option.id ?? '') === value} class:active={index === active} class:chosen={(option.id ?? '') === value}
          onmousedown={() => pick(option)} onmouseenter={() => active = index}>{option.text}</button>
      {/each}
    </div>
  {/if}
</span>

<style>
  .search-select{position:relative;display:block;min-width:0}
  input{box-sizing:border-box;min-width:0;width:100%;padding:6px 18px 6px 6px;border:1px solid var(--ui-line-strong,#414139);border-radius:2px;background:var(--ui-surface-2,#20221d);color:var(--ui-gold-bright,#ccc6b2);font:inherit;font-size:10px}
  input::placeholder{color:var(--ui-muted,#8a846f)}
  .caret{position:absolute;right:6px;top:50%;transform:translateY(-50%);pointer-events:none;color:var(--ui-muted,#8a846f);font-size:9px}
  .options{position:absolute;z-index:30;top:100%;left:0;right:0;max-height:260px;overflow:auto;border:1px solid var(--ui-line-strong,#555042);background:var(--ui-sunk,#111310);box-shadow:0 10px 30px var(--ui-shadow,#000)}
  button{display:block;width:100%;padding:6px 7px;border:0;border-bottom:1px solid var(--ui-line,#282a25);background:transparent;color:var(--ui-gold-bright,#c3bda8);text-align:left;font:inherit;font-size:10px;cursor:pointer}
  button.active{background:var(--ui-hover,#25271f)}
  button.chosen{color:var(--gold-bright,#e2c675)}
</style>
