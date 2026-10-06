import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import vm from 'node:vm'
import test from 'node:test'

const content = readFileSync(new URL('../../browser-extension/content.js', import.meta.url), 'utf8')
const background = readFileSync(new URL('../../browser-extension/background.js', import.meta.url), 'utf8')
const code = 'code-0123456789abcdef'
const site = 'https://www.pathofexile.com'

function page({ loggedOut = false, links = [], loading = false, hash = '' } = {}) {
  const messages = []
  let loaded
  const bar = { querySelector: () => loggedOut ? {} : null, querySelectorAll: () => links }
  vm.runInNewContext(content, {
    URL, location: { hash, origin: site, href: site + '/trade2', pathname: '/trade2', search: '' },
    history: { replaceState() {} }, chrome: { runtime: { sendMessage: m => messages.push(m) } },
    document: { readyState: loading ? 'loading' : 'complete', getElementById: () => bar,
      addEventListener: (_, cb) => { loaded = cb } },
  })
  return { messages, loaded }
}

test('reads the account discriminator only from the account bar after DOM ready', () => {
  const p = page({ loading: true, hash: '#mrw-link=' + code,
    links: [{ href: site + '/my-account', textContent: ' MrW#1234 ' }] })
  assert.equal(p.messages.length, 0)
  p.loaded()
  assert.deepEqual(JSON.parse(JSON.stringify(p.messages)), [{ type: 'mrw-link', code, accountName: 'MrW#1234' }])
  assert.equal(page({ links: [{ href: site + '/account/view-profile/Player%231234', textContent: 'Player#1234' }] }).messages[0].accountName, 'Player#1234')
})

test('signed-out header and foreign profile links never identify an account', () => {
  const link = { href: site + '/account/view-profile/Other', textContent: 'Other#1234' }
  assert.equal(page({ loggedOut: true, links: [link] }).messages[0].accountName, '')
  assert.equal(page({ links: [{ ...link, href: 'https://evil.example/account/view-profile/Other' }] }).messages[0].accountName, '')
})

function bridge({ cookie = true, signedIn = true } = {}) {
  let listener
  const sent = []
  const storage = {}
  const api = { runtime: { id: 'test-extension', getManifest: () => ({ version: '1.2.0' }),
    onMessage: { addListener: cb => { listener = cb } } },
    cookies: { get: async () => cookie ? { value: 'fake-cookie-never-real' } : null },
    storage: { session: { set: async v => Object.assign(storage, v), get: async () => storage,
      remove: async k => { delete storage[k] } } } }
  vm.runInNewContext(background, { URL, chrome: api, fetch: async (url, opt) => {
    if (url.startsWith(site)) return { ok: signedIn, type: signedIn ? 'basic' : 'opaqueredirect' }
    sent.push({ url, body: JSON.parse(opt.body) })
    return { ok: true, status: 204 }
  } })
  return { sent, storage, message: async (msg, sender = { id: api.runtime.id, url: site + '/trade2' }) => {
    listener(msg, sender)
    await new Promise(resolve => setImmediate(resolve))
  } }
}

test('login finishes the pending code with cookie and account name; code is spent', async () => {
  const b = bridge()
  await b.message({ type: 'mrw-link', code, accountName: '' })
  assert.equal(b.sent[0].body.state, 'no-session')
  assert.equal(b.storage.code, code)
  await b.message({ type: 'mrw-page', accountName: 'MrW#1234' })
  assert.equal(b.sent[1].body.accountName, 'MrW#1234')
  assert.equal(b.sent[1].body.session, 'fake-cookie-never-real')
  assert.equal(b.storage.code, undefined)
  await b.message({ type: 'mrw-page', accountName: 'Other#5678' })
  assert.equal(b.sent.length, 2)
})

test('missing cookie, redirect and foreign sender cannot link an account', async () => {
  for (const options of [{ cookie: false }, { signedIn: false }]) {
    const b = bridge(options)
    await b.message({ type: 'mrw-link', code, accountName: 'MrW#1234' })
    assert.equal(b.sent[0].body.session, undefined)
    assert.equal(b.sent[0].body.state, 'no-session')
  }
  const b = bridge()
  await b.message({ type: 'mrw-link', code, accountName: 'MrW#1234' }, { id: 'test-extension', url: 'https://evil.example' })
  assert.equal(b.sent.length, 0)
})
