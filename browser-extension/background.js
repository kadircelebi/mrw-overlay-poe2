// Hands the pathofexile.com session to the MrW Overlay app, and only
// when the app asked for it: the app opens pathofexile.com with a one-time
// link code, and only a request carrying that code is accepted by the app.
// The session goes to this computer (127.0.0.1) and nowhere else.
const api = globalThis.browser ?? globalThis.chrome
const APP = 'http://127.0.0.1:47819'
const SITE = 'https://www.pathofexile.com/'
const CODE = /^[A-Za-z0-9_-]{16,64}$/

// pathofexile.com gives signed-out visitors a POESESSID too, so the cookie
// alone proves nothing. The account page redirects signed-out visitors to
// the sign-in page; a direct answer means the session is signed in.
async function signedIn() {
  try {
    const res = await fetch(SITE + 'my-account', { redirect: 'manual', credentials: 'include' })
    return res.type !== 'opaqueredirect' && res.ok
  } catch {
    return false
  }
}

async function report(code, accountName = '') {
  const cookie = await api.cookies.get({ url: SITE, name: 'POESESSID' })
  const ready = !!cookie && !!accountName && (await signedIn())
  const version = api.runtime.getManifest().version
  const body = ready ? { code, version, session: cookie.value, accountName } : { code, version, state: 'no-session' }
  try {
    const res = await fetch(`${APP}/${ready ? 'link' : 'status'}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    })
    // Once the app has the session the code is spent; a code the app no
    // longer knows (409) is dropped as well.
    if ((ready && res.ok) || res.status === 409) await api.storage.session.remove('code')
  } catch {
    // The app is closed or no longer waiting; nothing to do.
  }
}

async function pendingCode() {
  const { code } = await api.storage.session.get('code')
  return code
}

api.runtime.onMessage.addListener((message, sender) => {
  if (sender.id !== api.runtime.id) return
  try { if (new URL(sender.url).origin !== new URL(SITE).origin) return } catch { return }
  const accountName = typeof message?.accountName === 'string' ? message.accountName.trim() : ''
  if (accountName.length > 128 || /[\u0000-\u001f\u007f]/.test(accountName)) return
  if (message?.type === 'mrw-link' && CODE.test(message.code ?? '')) {
    api.storage.session.set({ code: message.code }).then(() => report(message.code, accountName))
  } else if (message?.type === 'mrw-page') {
    // A page after signing in: finish a connection that is still waiting.
    pendingCode().then((code) => { if (code) report(code, accountName) })
  }
})

// A cookie change alone cannot tell us the account name. The content script
// reports the account bar on the page shown after sign-in instead.
