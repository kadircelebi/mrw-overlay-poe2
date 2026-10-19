// Dragging a box over the game into place from its own page: the pointer is
// held by the box while the button is down, and each move asks the app to
// move the window by how far the pointer went (in physical pixels). Windows'
// own move loop is not used: the boxes never take the focus, and a move
// started after the button was already up would leave Windows moving the
// box with every click going to it. Buttons inside the box stay buttons.
// With keep (a box's grip), letting go also keeps the box's new place.
import { AppService } from '../../bindings/poe2filter/internal/app'

let drags = 0

export function boxDrag(node: HTMLElement, opts: { box: string; keep?: boolean }) {
  const { box, keep } = opts
  let start: { x: number; y: number; id: number } | null = null
  let seq = 0
  let frame = 0
  let latest = { dx: 0, dy: 0 }

  function send() {
    frame = 0
    if (start) void AppService.MoveBox(box, start.id, ++seq, latest.dx, latest.dy)
  }

  function down(e: PointerEvent) {
    if (e.button !== 0 || (e.target as HTMLElement).closest('button')) return
    e.preventDefault()
    node.setPointerCapture(e.pointerId)
    // Unique across the boxes' and grips' pages.
    start = { x: e.screenX, y: e.screenY, id: Date.now() * 10 + (++drags % 10) }
    seq = 0
  }

  function move(e: PointerEvent) {
    if (!start) return
    const ratio = window.devicePixelRatio || 1
    latest = { dx: Math.round((e.screenX - start.x) * ratio), dy: Math.round((e.screenY - start.y) * ratio) }
    if (!frame) frame = requestAnimationFrame(send)
  }

  function up(e: PointerEvent) {
    if (!start) return
    move(e)
    if (frame) cancelAnimationFrame(frame)
    frame = 0
    if (keep) void AppService.EndBoxMove(box, start.id, ++seq, latest.dx, latest.dy)
    else send()
    start = null
    if (node.hasPointerCapture(e.pointerId)) node.releasePointerCapture(e.pointerId)
  }

  node.addEventListener('pointerdown', down)
  node.addEventListener('pointermove', move)
  node.addEventListener('pointerup', up)
  node.addEventListener('pointercancel', up)
  return {
    destroy() {
      node.removeEventListener('pointerdown', down)
      node.removeEventListener('pointermove', move)
      node.removeEventListener('pointerup', up)
      node.removeEventListener('pointercancel', up)
      if (frame) cancelAnimationFrame(frame)
    },
  }
}
