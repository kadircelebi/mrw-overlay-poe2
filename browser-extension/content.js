// Runs on pathofexile.com. The app opens this site with a one-time link code
// in the address fragment (#mrw-link=...); the fragment never reaches GGG's
// server. The code is passed to the background script and removed from the
// address bar. Every other page load is reported too, so a connection that is
// waiting for a sign-in completes on the page shown after signing in.
const api = globalThis.browser ?? globalThis.chrome
const match = location.hash.match(/mrw-link=([A-Za-z0-9_-]{16,64})/)
if (match) {
  history.replaceState(null, '', location.pathname + location.search)
}

// Only the signed-in account in the site's top bar is ours. Profile links
// elsewhere (forum authors, ladders) must never become the connected account.
function accountName() {
  const bar = document.getElementById('statusBar')
  if (!bar || bar.querySelector('.loggedOut')) return ''
  for (const link of bar.querySelectorAll('a[href]')) {
    const url = new URL(link.href, location.href)
    if (url.origin !== location.origin || !(/^\/account\/(?:xbox\/|sony\/)?view-profile\//.test(url.pathname) || /^\/my-account\/?$/.test(url.pathname))) continue
    const name = link.textContent.trim()
    if (name && name.length <= 128 && !/[\u0000-\u001f\u007f]/.test(name)) return name
  }
  return ''
}

function reportPage() {
  api.runtime.sendMessage({ type: match ? 'mrw-link' : 'mrw-page',
    ...(match ? { code: match[1] } : {}), accountName: accountName() })
}

// document_start is too early to read the account bar. Read it after the
// login page has finished navigating and its DOM is available.
if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', reportPage, { once: true })
else reportPage()
