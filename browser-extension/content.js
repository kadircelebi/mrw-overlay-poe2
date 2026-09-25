// Runs on pathofexile.com. The app opens this site with a one-time link code
// in the address fragment (#mrw-link=...); the fragment never reaches GGG's
// server. The code is passed to the background script and removed from the
// address bar. Every other page load is reported too, so a connection that is
// waiting for a sign-in completes on the page shown after signing in.
const api = globalThis.browser ?? globalThis.chrome
const match = location.hash.match(/mrw-link=([A-Za-z0-9_-]{16,64})/)
if (match) {
  api.runtime.sendMessage({ type: 'mrw-link', code: match[1] })
  history.replaceState(null, '', location.pathname + location.search)
} else {
  api.runtime.sendMessage({ type: 'mrw-page' })
}
