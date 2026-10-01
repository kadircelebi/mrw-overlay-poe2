import { AppService } from '../../bindings/poe2filter'

let pending: Promise<void> = Promise.resolve()
// Preserve focus/blur order when switching rapidly between shortcut fields.
export function setHotkeyCapture(active: boolean): Promise<void> {
  pending = pending.catch(() => {}).then(() => AppService.SetHotkeyCapture(active))
  return pending
}
