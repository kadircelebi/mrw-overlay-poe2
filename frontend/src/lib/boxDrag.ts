// Dragging a box over the game into place: pressing the button on the node
// starts the drag, and the app moves the box with the pointer until the
// button is let go. The page does not follow the pointer itself: a window
// behind the game gets the mouse only while the pointer is over it, so a
// small grip lost it on a quick move, and the button was let go over the
// game. Buttons inside the box stay buttons. Let go of by a grip, the box
// keeps its new place.
import { AppService } from '../../bindings/poe2filter/internal/app'

export function boxDrag(node: HTMLElement, opts: { box: string }) {
  const { box } = opts

  function down(e: PointerEvent) {
    if (e.button !== 0 || (e.target as HTMLElement).closest('button')) return
    e.preventDefault()
    void AppService.DragBox(box)
  }

  node.addEventListener('pointerdown', down)
  return {
    destroy() {
      node.removeEventListener('pointerdown', down)
    },
  }
}
