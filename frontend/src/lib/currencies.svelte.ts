import { AppService } from '../../bindings/poe2filter'
import type { CurrencyEntry } from '../../bindings/poe2filter/internal/overlay/models'

// The trade site's currency list (name and icon by the id a listing is priced
// in), loaded once per window on first use.
let byId = $state<Record<string, CurrencyEntry>>({})
let requested = false

function load() {
  if (requested) return
  requested = true
  AppService.TradeCurrencies()
    .then((list) => { byId = Object.fromEntries((list ?? []).map((entry) => [entry.id, entry])) })
    .catch(() => { requested = false })
}

export function currencyInfo(id: string): CurrencyEntry | undefined {
  load()
  return byId[id]
}
