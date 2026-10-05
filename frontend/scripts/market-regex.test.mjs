import { test } from 'node:test'
import assert from 'node:assert/strict'
import { buildMarketRegex, integerRange, modifierPattern } from '../src/lib/marketRegex.ts'

const texts = ['#% increased Pack Size in Map', '#% increased chance for Desecrated Currency from Abysses in Map',
  'Monsters have #% increased Effectiveness', 'Map has #% increased number of Rare Monsters', 'Map contains an additional Abyss']
const rivals = ['#% reduced Pack Size in Map', 'Breaches in Map have #% increased Pack Size',
  'Abyssal Monsters have #% increased Effectiveness for each closed Pit, up to 100%', 'Map contains an additional Shrine']
const catalogue = [...texts, ...rivals]
const choices = texts.map((text, i) => ({ selected: true, mod: { text, key: String(i), statId: 'explicit.' + i, type: 'explicit' } }))
const input = { choices, groups: [{ type: 'and', choiceKeys: choices.map(c => c.mod.key) }], filters: [], catalogue }
function matches(query, lines) {
  return [...query.matchAll(/"([^"]+)"/g)].every(([,pattern]) => {
    const negative = pattern.startsWith('!')
    const found = lines.some(line => new RegExp(negative ? pattern.slice(1) : pattern, 'i').test(line))
    return negative ? !found : found
  })
}
const tablet = ['5% increased Pack Size in Map', '21% increased chance for Desecrated Currency from Abysses in Map',
  'Monsters have 14% increased Effectiveness', 'Map has 31% increased number of Rare Monsters', 'Map contains an additional Abyss']

test('tablet all requires every selected modifier; any accepts each one independently', () => {
  const all = buildMarketRegex({ ...input, mode: 'all' }), any = buildMarketRegex({ ...input, mode: 'any' })
  assert.equal(all.error, ''); assert.equal(any.error, '')
  assert.ok(matches(all.text, tablet)); assert.ok(matches(any.text, tablet))
  for (let i = 0; i < tablet.length; i++) {
    assert.ok(!matches(all.text, tablet.filter((_, j) => i !== j)))
    assert.ok(matches(any.text, [tablet[i]]))
  }
  for (const other of rivals) assert.ok(!matches(any.text, [other.replaceAll('#', '5')]))
})

test('numbers are ignored for affix presence; unchecked and removed rows stay out', () => {
  const result = buildMarketRegex({ ...input, choices: [{ ...choices[0], min: 25 }, { ...choices[1], selected: false },
    { ...choices[2], mod: { ...choices[2].mod, key: 'removed' } }], mode: 'all' })
  assert.ok(matches(result.text, ['1% increased Pack Size in Map']))
  assert.equal(result.omitted.length, 0)
  assert.equal(result.ignoredAffixBounds, true)
  assert.equal([...result.text.matchAll(/"/g)].length, 2)
})

test('Area wording matches Map wording and regex metacharacters are escaped', () => {
  const pattern = modifierPattern('Area contains an additional Abyss', catalogue)
  for (const text of ['Map contains an additional Abyss', 'Area contains an additional Abyss']) assert.ok(new RegExp(pattern,'i').test(text))
  assert.ok(new RegExp(modifierPattern('Grants Skill: Fire (Greater)', []),'i').test('Grants Skill: Fire (Greater)'))
})

test('integer bounds are exact across tens and digit counts', () => {
  for (const [lo, hi] of [[0,9],[5,25],[21,45],[31,99],[95,105],[120,250],[0,9999]]) {
    const re = new RegExp('^' + integerRange(lo,hi) + '$')
    for (let n = 0; n <= Math.min(10000,hi+2); n++) assert.equal(re.test(String(n)), n >= lo && n <= hi, `${lo}-${hi}: ${n}`)
  }
  assert.equal(integerRange(50,30),null); assert.equal(integerRange(1.5,10),null)
})

test('waystone summary and corruption are required even in any mode, without rounding', () => {
  const result = buildMarketRegex({ ...input, mode: 'any', filters: [
    { group:'map_filters',id:'map_magic_monsters',min:35 }, { group:'map_filters',id:'map_rare_monsters',min:30,max:100 },
    { group:'misc_filters',id:'corrupted',option:'true' }] })
  const summaries = ['Monster Effectiveness: +35%', 'Monster Rarity: +30%', 'Corrupted', tablet[0]]
  assert.equal(result.error,''); assert.ok(matches(result.text,summaries))
  assert.ok(!matches(result.text,[...summaries.filter(s=>!s.includes('Effectiveness')), 'Monster Effectiveness: +34%']))
  assert.ok(!matches(result.text,summaries.filter(s=>s !== 'Corrupted')))
  assert.ok(!matches(result.text,[...summaries.filter(s=>!s.includes('Rarity')), 'Monster Rarity: +101%']))
})

test('tablet uses minimum, negative mods and item title remain independent of any', () => {
  const uses = {selected:true,min:10,mod:{text:'# uses remaining (Tablets)',key:'uses',statId:'pseudo.uses',type:'pseudo'}}
  const result = buildMarketRegex({ ...input, choices: [choices[0],choices[4],uses], groups:[
    {type:'and',choiceKeys:['0','uses']},{type:'not',choiceKeys:['4']}], baseType:'Abyss Tablet', mode:'any' })
  assert.ok(matches(result.text,['Abyss Tablet', tablet[0], '10 uses remaining']))
  assert.ok(!matches(result.text,['Abyss Tablet', tablet[0], '6 uses remaining']))
  assert.ok(!matches(result.text,['Abyss Tablet',tablet[0],tablet[4],'10 uses remaining']))
  const noMinimum = buildMarketRegex({...input,choices:[{...uses,min:undefined}],groups:[{type:'and',choiceKeys:['uses']}],mode:'all'})
  assert.ok(matches(noMinimum.text,['8 uses remaining']))
})

test('unsupported pseudo and trade filters are reported; overlong output is not copyable', () => {
  const unsupported = {selected:true,mod:{text:'Total Resistance',key:'sum',statId:'pseudo.sum',type:'pseudo'}}
  const result=buildMarketRegex({...input,choices:[unsupported],groups:[{type:'and',choiceKeys:['sum']}],filters:[{group:'trade_filters',id:'price',min:1,label:'Price'}],mode:'all'})
  assert.equal(result.error,'empty'); assert.deepEqual(result.omitted,['Total Resistance','Price'])
  assert.equal(buildMarketRegex({...input,baseType:'a'.repeat(251),mode:'all'}).error,'long')
})

test('clearing numeric bounds removes the warning, including empty invalid/null values', () => {
  const choice = { ...choices[0], min: 10, max: 30 }
  const make = () => buildMarketRegex({ ...input, choices: [choice], mode: 'all' })
  assert.equal(make().ignoredAffixBounds, true)
  for (const blank of [undefined, null, NaN]) {
    choice.min = blank; choice.max = blank
    assert.equal(make().ignoredAffixBounds, false)
    assert.deepEqual(make().omitted, [])
  }
})

test('uses copied with an actual number keeps its threshold and has no omitted-affix warning', () => {
  const uses = { selected: true, min: 10, mod: { text: '6 uses remaining', key: 'uses', statId: 'explicit.uses', type: 'explicit' } }
  const result = buildMarketRegex({ ...input, choices: [uses], groups: [{ type: 'and', choiceKeys: ['uses'] }], mode: 'all' })
  assert.equal(result.ignoredAffixBounds, false)
  assert.deepEqual(result.omitted, [])
  assert.ok(matches(result.text, ['10 uses remaining']))
  assert.ok(!matches(result.text, ['6 uses remaining']))
})
