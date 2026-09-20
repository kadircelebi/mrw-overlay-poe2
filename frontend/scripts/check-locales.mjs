// Every locale must cover exactly the English keys: a typo in one table would
// otherwise fall back to English forever without anyone noticing.
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, join } from 'node:path'

const dir = join(dirname(fileURLToPath(import.meta.url)), '..', 'src', 'lib', 'locales')
const keys = (file) =>
  [...readFileSync(join(dir, file), 'utf8').matchAll(/^ {2}'([^']+)':/gm)].map((m) => m[1])

const en = keys('en.ts')
const enSet = new Set(en)
let bad = false

for (const file of ['tr.ts', 'zh.ts']) {
  const own = new Set(keys(file))
  const missing = en.filter((k) => !own.has(k))
  const unknown = [...own].filter((k) => !enSet.has(k))
  if (missing.length) {
    console.error(`${file}: missing ${missing.length} key(s): ${missing.join(', ')}`)
    bad = true
  }
  if (unknown.length) {
    console.error(`${file}: unknown key(s) not in en.ts: ${unknown.join(', ')}`)
    bad = true
  }
}

if (bad) process.exit(1)
console.log(`locales ok (${en.length} keys)`)
