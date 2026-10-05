<script lang="ts">
  import { onDestroy } from 'svelte'
  import { t } from './i18n.svelte'
  import type { RegexResult } from './marketRegex'

  let { all, any }: { all: RegexResult; any: RegexResult } = $props()
  let open = $state(false)
  let feedback = $state('')
  let failed = $state(false)
  let copying = $state(false)
  let host: HTMLDivElement
  let timer: ReturnType<typeof setTimeout> | undefined
  const omissions = $derived([...new Set([...all.omitted, ...any.omitted])])

  function closeOutside(event: PointerEvent) {
    if (host && !host.contains(event.target as Node)) open = false
  }

  async function copy(result: RegexResult) {
    if (copying || result.error) return
    copying = true
    failed = false
    try {
      await navigator.clipboard.writeText(result.text)
      open = false
      feedback = t('mk.regexCopied')
    } catch {
      failed = true
      feedback = t('mk.regexCopyFailed')
    } finally {
      copying = false
      clearTimeout(timer)
      timer = setTimeout(() => { feedback = '' }, 3500)
    }
  }
  onDestroy(() => clearTimeout(timer))
</script>

<svelte:window onpointerdown={closeOutside} onkeydown={(event) => { if (event.key === 'Escape') open = false }} />
<div class="regex-copy" bind:this={host}>
  <button class="regex-trigger" title={t('mk.regexHint')} aria-label={t('mk.regexHint')} aria-expanded={open} aria-controls="market-regex-options" onclick={() => { open = !open; feedback = '' }}>
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true"><rect x="8" y="8" width="12" height="13" rx="1" /><path d="M16 8V3H3v13h5" /></svg>
  </button>
  {#if open}
    <div class="regex-menu" id="market-regex-options">
      <button disabled={!!all.error || copying} title={all.error ? t('mk.regex' + (all.error === 'long' ? 'Long' : 'Empty')) : undefined} onclick={() => copy(all)}>{t('mk.regexAll')}</button>
      <button disabled={!!any.error || copying} title={any.error ? t('mk.regex' + (any.error === 'long' ? 'Long' : 'Empty')) : undefined} onclick={() => copy(any)}>{t('mk.regexAny')}</button>
      {#if all.error || any.error}<p>{t('mk.regex' + (all.error === 'long' || any.error === 'long' ? 'Long' : 'Empty'))}</p>{/if}
      {#if all.ignoredAffixBounds || any.ignoredAffixBounds}<p class="affix-bounds">{t('mk.regexAffixBounds')}</p>{/if}
      {#if omissions.length}<p class="omissions">{t('mk.regexOmitted')} {omissions.join(' · ')}</p>{/if}
    </div>
  {/if}
  {#if feedback}<div class="feedback" class:failed role="status">{feedback}</div>{/if}
</div>

<style>
  .regex-copy{position:relative;display:flex}
  .regex-trigger{display:flex;align-items:center;justify-content:center;width:36px;border:1px solid var(--ui-line-strong,#8c7b50);background:var(--ui-surface,#171917);color:var(--gold-bright)}
  .regex-trigger:hover,.regex-trigger[aria-expanded="true"]{background:var(--ui-hover,#25261f)}
  .regex-trigger:focus-visible,.regex-menu button:focus-visible{outline:2px solid var(--gold-bright);outline-offset:2px}
  .regex-menu{position:absolute;z-index:30;top:calc(100% + 4px);right:0;width:220px;padding:4px;border:1px solid var(--ui-line-strong,#8c7b50);background:var(--ui-surface,#171917);box-shadow:0 8px 20px var(--ui-shadow,#0009)}
  .regex-menu button{display:block;width:100%;padding:9px 10px;border:0;background:none;color:var(--gold-bright);text-align:left;font-size:11px}
  .regex-menu button:hover:not(:disabled){background:var(--ui-hover,#2b2d23)}
  .regex-menu button:disabled{opacity:.45}
  .regex-menu p{margin:5px 7px;color:var(--ui-gold-bright,#d9924a);font-size:10px;line-height:1.4;overflow-wrap:anywhere}
  .omissions{max-height:130px;overflow:auto}
  .feedback{position:absolute;z-index:30;top:calc(100% + 4px);right:0;min-width:145px;padding:9px;border:1px solid var(--ui-line-strong,#6f9c6c);background:var(--ui-surface,#171917);color:var(--ui-ok,#a5c599);font-size:11px}
  .feedback.failed{border-color:var(--ui-line-strong,#ba7065);color:var(--ui-bad,#e2a095)}
</style>
