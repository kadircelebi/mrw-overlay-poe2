// The overlay windows have no language setting of their own: they follow the
// one picked in Settings, and switch along when it changes.
//
// The document itself stays lang="en" here. Much of what these windows show
// is the game's own English (Item Level, Prefix, Corrupted), and CSS
// uppercasing under lang="tr" would turn its "i" into "İ".
import { Events } from '@wailsio/runtime'
import { AppService } from '../../bindings/poe2filter'
import type { Config } from '../../bindings/poe2filter/internal/filter/models'
import { setLang } from './i18n.svelte'

async function apply(setting?: string) {
  const value = setting ?? (await AppService.GetConfig()).language
  if (value && value !== 'auto') {
    setLang(value)
    return
  }
  const languages = (await AppService.Languages()) ?? []
  setLang(languages.find((l) => l.auto)?.resolved ?? 'en')
}

/** Applies the app's language now and on every change; returns the unsubscribe. */
export function followAppLanguage(): () => void {
  apply().catch(() => {})
  return Events.On('config', (event) => {
    apply((event.data as Config).language).catch(() => {})
  })
}
