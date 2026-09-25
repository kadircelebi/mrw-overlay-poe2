// The mouse wheel steps number inputs up and down, as in POE2 Overlay.
//
// A wheel gesture that began as a scroll keeps scrolling when the pointer
// passes over an input, so running down the Market's long filter list does
// not rewrite values on the way. Turning the wheel again over the input (or
// over the focused input at any time) steps it. Shift steps ten times as far.

const scrollGrace = 350 // ms without wheel events that ends a scroll gesture

function stepFor(input: HTMLInputElement): number {
  const declared = Number(input.step)
  if (input.step && input.step !== 'any' && Number.isFinite(declared) && declared > 0) return declared
  // Decimal values (attack speed, crit) move by a tenth; everything else by one.
  const value = input.value.trim()
  return value.includes('.') && Math.abs(Number(value)) < 10 ? 0.1 : 1
}

function decimalsOf(n: number): number {
  const text = String(n)
  const dot = text.indexOf('.')
  return dot < 0 ? 0 : text.length - dot - 1
}

export function installWheelNumbers(root: Document = document) {
  let lastScroll = -Infinity
  root.addEventListener('wheel', (event) => {
    const now = performance.now()
    const input = (event.target as Element | null)?.closest?.('input[type="number"]') as HTMLInputElement | null
    const scrolling = now - lastScroll < scrollGrace
    if (!input || input.disabled || input.readOnly || event.deltaY === 0 || event.ctrlKey || (scrolling && root.activeElement !== input)) {
      lastScroll = now
      return
    }
    event.preventDefault()
    const step = stepFor(input) * (event.shiftKey ? 10 : 1)
    const empty = input.value.trim() === ''
    // An empty box means "no limit"; only an upward turn starts it (from 0).
    if (empty && event.deltaY > 0) return
    const current = empty ? 0 : Number(input.value)
    if (!Number.isFinite(current)) return
    let next = current + (event.deltaY < 0 ? step : -step)
    const min = input.min === '' ? NaN : Number(input.min)
    const max = input.max === '' ? NaN : Number(input.max)
    if (Number.isFinite(min)) next = Math.max(min, next)
    if (Number.isFinite(max)) next = Math.min(max, next)
    const places = Math.max(decimalsOf(step), decimalsOf(current))
    const text = String(Number(next.toFixed(places)))
    if (text === input.value) return
    // Go through the native setter and real events so Svelte bindings and
    // oninput/onchange handlers see the change exactly as if it were typed.
    Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')!.set!.call(input, text)
    input.dispatchEvent(new Event('input', { bubbles: true }))
    input.dispatchEvent(new Event('change', { bubbles: true }))
  }, { passive: false })
}
