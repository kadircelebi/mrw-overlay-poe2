<script lang="ts">
  import { onDestroy } from 'svelte'
  import { setHotkeyCapture } from './hotkeyCapture'
  import { t } from './i18n.svelte'
  import { recordedHotkey } from './hotkey'
  // compact (for lists) shows the hint only while a key is awaited.
  let { value = $bindable(''), label, onchange, compact = false }: { value: string; label: string; onchange: () => void; compact?: boolean } = $props()
  let ready = $state(false)
  let error = $state('')
  let recording = false
  let input: HTMLInputElement
  async function begin() {
    recording = true
    error = ''
    try { await setHotkeyCapture(true); ready = recording } catch (e) { error = String(e) }
  }
  function end() {
    recording = false
    ready = false
    void setHotkeyCapture(false).catch((e) => error = String(e))
  }
  function capture(event: KeyboardEvent) {
    if (event.key === 'Tab') return
    event.preventDefault()
    event.stopPropagation()
    if (event.key === 'Escape') { (event.currentTarget as HTMLInputElement).blur(); return }
    if (!ready) return
    const next = recordedHotkey(event)
    if (!next) return
    if (value !== next) { value = next; onchange() }
    ;(event.currentTarget as HTMLInputElement).blur()
  }
  onDestroy(() => { if (recording) end() })
</script>

<svelte:window onblur={() => { if (recording) { input.blur(); end() } }} />

<input bind:this={input} readonly {value} onfocus={begin} onblur={end} onkeydown={capture} aria-label={label} title={t('hotkey.record')} />
{#if !compact || ready}<span class="hint">{ready ? t('hotkey.listening') : t('hotkey.record')}</span>{/if}
{#if error}<span class="error">{error}</span>{/if}

<style>
  input:focus { border-color: var(--gold-bright); }
  .hint { color: var(--muted); font-size: 11px; }
  .error { color: var(--danger); }
</style>
