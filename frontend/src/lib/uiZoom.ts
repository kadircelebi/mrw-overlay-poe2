import { Events } from '@wailsio/runtime'
import { AppService } from '../../bindings/poe2filter'

// The in-game windows' UI size below 100%. WebView2's zoom cannot go under 1
// through Wails, so the Go side sends the remaining factor and the page
// zooms itself with CSS.
let current = 1

function apply(value: unknown) {
  const z = Number(value)
  current = Number.isFinite(z) && z > 0 && z <= 1 ? z : 1
  document.documentElement.style.zoom = current === 1 ? '' : String(current)
}

export function followUIZoom(window: string) {
  AppService.UIZoom(window).then(apply).catch(() => {})
  // Every window hears every 'ui-zoom'; only this window's applies.
  Events.On('ui-zoom', (event) => {
    const data = event.data as { window?: string; zoom?: number } | null
    if (data?.window === window) apply(data.zoom)
  })
}
