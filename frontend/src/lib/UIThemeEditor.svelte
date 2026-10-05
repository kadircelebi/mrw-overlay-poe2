<script lang="ts">
  import { onMount } from 'svelte'
  import { AppService } from '../../bindings/poe2filter/internal/app'
  import { uiThemes, acceptUIThemes, themeFields, type UITheme } from './uiTheme'
  import { t } from './i18n.svelte'
  const selected=$derived($uiThemes.themes.find(x=>x.id===$uiThemes.selected))
  let busy=$state(false), error=$state(''), saved=$state(false), copying=$state(false), deleting=$state(false)
  let name=$state(''), draft=$state<UITheme|null>(null)
  const groups=[
    ['surfaces',['bg','surface','surface-2','surface-3','sunk','hover']],
    ['text',['text','text-2','muted','gold','gold-bright','gold-dim','on-accent']],
    ['borders',['line','line-strong','ok','warn','bad','exceptional']],
    ['item',['item-bg','item-line','item-divider','item-text','item-muted','item-property','item-outline']],
  ] as const
  function copy(theme:UITheme) { return {...theme,colors:{...theme.colors}} }
  async function run(action:()=>PromiseLike<unknown>) {
    busy=true;error='';saved=false
    try { return acceptUIThemes(await action()) }
    catch(e) { error=String(e).replace(/^RuntimeError:\s*/i,''); return null }
    finally {busy=false}
  }
  async function select(id:string) { if(await run(()=>AppService.SelectUITheme(id))) {draft=null;copying=false;deleting=false} }
  async function create(event:SubmitEvent) {
    event.preventDefault(); if(!selected || !name.trim())return
    const next=await run(()=>AppService.CreateUITheme(selected.id,name.trim()))
    if(next) { const theme=next.themes.find(t=>t.id===next.selected)!; draft=copy(theme); copying=false;name='' }
  }
  async function save(event:SubmitEvent) { event.preventDefault();if(!draft)return;const next=await run(()=>AppService.SaveUITheme(draft!));if(next){draft=null;saved=true} }
  async function remove() { if(!selected)return; if(await run(()=>AppService.DeleteUITheme(selected.id))){draft=null;deleting=false} }
  onMount(()=>{void AppService.GetUIThemes().then(acceptUIThemes).catch(e=>{error=String(e)})})
</script>

<section class="theme-settings" aria-label={t('uiTheme.title')}>
  <h2>{t('uiTheme.title')}</h2>
  <p>{t('uiTheme.hint')}</p>
  <label class="theme-choice"><span>{t('uiTheme.selected')}</span><select value={$uiThemes.selected} disabled={busy} onchange={e=>void select(e.currentTarget.value)}>
    {#each $uiThemes.themes as theme (theme.id)}<option value={theme.id}>{theme.name}{theme.readOnly ? '' : ` · ${t('uiTheme.custom')}`}</option>{/each}
  </select></label>
  <div class="theme-actions">
    <button disabled={busy} onclick={()=>{copying=!copying;draft=null;deleting=false;name=selected ? `${selected.name} ${t('uiTheme.copySuffix')}` : ''}}>{t('uiTheme.derive')}</button>
    {#if selected && !selected.readOnly}
      <button disabled={busy} onclick={()=>{draft=copy(selected!);copying=false;deleting=false;saved=false}}>{t('uiTheme.edit')}</button>
      <button class="delete" disabled={busy} onclick={()=>{deleting=true;draft=null;copying=false}}>{t('uiTheme.delete')}</button>
    {/if}
  </div>
  {#if selected?.readOnly}<p class="hint">{t('uiTheme.locked')}</p>{/if}
  {#if copying}<form onsubmit={create} class="copy-form"><label><span>{t('uiTheme.name')}</span><input bind:value={name} required maxlength="60" disabled={busy} /></label><button disabled={busy || !name.trim()}>{t('uiTheme.create')}</button><button type="button" disabled={busy} onclick={()=>copying=false}>{t('uiTheme.cancel')}</button></form>{/if}
  {#if deleting}<div class="delete-confirm"><p>{t('uiTheme.deleteConfirm',selected?.name??'')}</p><button class="delete" disabled={busy} onclick={remove}>{t('uiTheme.delete')}</button><button disabled={busy} onclick={()=>deleting=false}>{t('uiTheme.cancel')}</button></div>{/if}
  {#if draft}<form onsubmit={save} class="editor">
    <label class="theme-name"><span>{t('uiTheme.name')}</span><input bind:value={draft.name} required maxlength="60" disabled={busy} /></label>
    {#each groups as [group,keys]}<fieldset disabled={busy}><legend>{t('uiTheme.group.'+group)}</legend><div class="colours">
      {#each keys as key}<label class="colour-field"><span>{t('uiTheme.color.'+key)}</span><input type="color" aria-label={t('uiTheme.color.'+key)} bind:value={draft.colors[key]} /><input aria-label={t('uiTheme.color.'+key)+' HEX'} bind:value={draft.colors[key]} pattern={'#[0-9a-fA-F]{6}'} required maxlength="7" spellcheck="false" /></label>{/each}
    </div></fieldset>{/each}
    <label class="grain"><input type="checkbox" bind:checked={draft.grain} disabled={busy} />{t('uiTheme.grain')}</label>
    <p class="hint">{t('uiTheme.fixed')}</p>
    <div class="theme-actions"><button disabled={busy || !draft.name.trim() || !themeFields.every(k=>/^#[\da-f]{6}$/i.test(draft!.colors[k]))}>{busy ? t('uiTheme.saving') : t('uiTheme.save')}</button><button type="button" disabled={busy} onclick={()=>draft=null}>{t('uiTheme.cancel')}</button></div>
  </form>{/if}
  {#if saved}<p class="success" role="status">{t('uiTheme.saved')}</p>{/if}
  {#if error}<p class="error" role="alert">{error}</p>{/if}
</section>

<style>
  .theme-settings{border:1px solid var(--line-strong);background:var(--surface);padding:16px;margin:0 0 16px;border-radius:var(--radius)}
  h2{font:700 16px var(--serif);color:var(--gold);margin:0 0 8px}p{color:var(--text-2);font-size:12px;margin:8px 0 12px}.hint{color:var(--muted);font-size:11px}
  .theme-choice,.theme-name{display:flex;align-items:center;gap:12px;margin:12px 0}.theme-choice select,.theme-name input{flex:1;min-width:0}
  input,select,button{font:inherit;border:1px solid var(--line-strong);border-radius:var(--radius-sm);padding:7px 9px;background:var(--sunk);color:var(--text)}button{background:var(--surface-2);color:var(--gold-bright);cursor:pointer;white-space:nowrap}button:hover:not(:disabled){border-color:var(--gold);background:var(--ui-hover,var(--surface-3))}button:active:not(:disabled){transform:translateY(1px)}button:disabled{opacity:.5;cursor:default}
  .theme-actions,.copy-form{display:flex;gap:8px;flex-wrap:wrap;align-items:end;margin:12px 0}.copy-form label{flex:1;min-width:180px}.copy-form input{width:100%;display:block;margin-top:5px}.delete{color:var(--bad)}.delete-confirm{border-top:1px solid var(--line);padding-top:8px}.delete-confirm button{margin-right:8px}
  .editor{border-top:1px solid var(--line);margin-top:16px;padding-top:8px}fieldset{border:0;border-top:1px solid var(--line);margin:16px 0;padding:12px 0 0;min-width:0}legend{color:var(--gold);font:700 12px var(--serif);padding:0 8px 0 0}.colours{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:8px 16px}.colour-field{display:grid;grid-template-columns:minmax(0,1fr) 32px 76px;gap:6px;align-items:center;font-size:11px}.colour-field input[type=color]{width:32px;height:30px;padding:2px;cursor:pointer}.colour-field input:not([type=color]){width:76px;padding:5px;font-size:11px}.grain{display:flex;gap:8px;align-items:center}.grain input{accent-color:var(--gold)}.error{color:var(--bad);overflow-wrap:anywhere}.success{color:var(--ok)}
  @media(max-width:900px){.colours{grid-template-columns:minmax(0,1fr)}}
</style>
