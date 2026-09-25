// Fails when a source file was saved in the wrong encoding. A file read as
// Windows-1252/1254 and written back as UTF-8 turns each Turkish letter or
// symbol into two or three Latin-1 characters (mojibake); this once broke the
// whole results panel.
import { execFileSync } from 'node:child_process'
import { readFileSync } from 'node:fs'
import { join } from 'node:path'

const root = execFileSync('git', ['rev-parse', '--show-toplevel'], { encoding: 'utf8' }).trim()
const files = execFileSync('git', ['ls-files', '--cached', '--others', '--exclude-standard'], { cwd: root, encoding: 'utf8' })
  .split('\n')
  .filter((file) => /\.(go|ts|svelte|css|html|js|mjs|json|md|yml|bat)$/.test(file) && !file.startsWith('frontend/bindings/'))

// Lead bytes of UTF-8 sequences as they look when decoded as a Windows code
// page: U+00C3/C4/C5 before a Latin-1 char (letters), U+00E2 before the
// cp1252 punctuation that UTF-8 continuation bytes become (arrows, dashes,
// shapes), U+00C2 before a symbol, and a stray byte-order mark.
const ch = (code) => String.fromCharCode(code)
const range = (from, to) => `${ch(from)}-${ch(to)}`
const latin1 = range(0x80, 0xff)
const mojibake = new RegExp([
  `${ch(0xc3)}[${latin1}${range(0x152, 0x178)}${range(0x2018, 0x203a)}]`,
  `${ch(0xc4)}[${latin1}]`,
  `${ch(0xc5)}[${latin1}${range(0x152, 0x178)}]`,
  `${ch(0xe2)}(?:${[0x20ac, 0x2020, 0x2021, 0x2013, 0x2014, 0x201e].map(ch).join('|')})`,
  `${ch(0xc2)}[${[0xb7, 0xb0, 0xb1, 0xa9, 0xae, 0xab, 0xbb, 0xa0].map(ch).join('')}]`,
  ch(0xfeff),
].join('|'))
const replacement = ch(0xfffd)
const problems = []
for (const file of files) {
  let bytes
  try { bytes = readFileSync(join(root, file)) } catch { continue }
  const text = bytes.toString('utf8')
  if (text.includes(replacement)) { problems.push(`${file}: not valid UTF-8`); continue }
  text.split('\n').forEach((line, index) => {
    if (mojibake.test(line)) problems.push(`${file}:${index + 1}: ${line.trim().slice(0, 90)}`)
  })
}
if (problems.length) {
  console.error('encoding check failed (mojibake or BOM):\n' + problems.join('\n'))
  process.exit(1)
}
console.log(`encoding ok (${files.length} files)`)
