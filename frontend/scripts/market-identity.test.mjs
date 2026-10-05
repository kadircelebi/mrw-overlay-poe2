import test from 'node:test'
import assert from 'node:assert/strict'
import { marketItemFromDraft, editMarketIdentity } from '../src/lib/marketIdentity.ts'

const item = { name: 'Viper Noose', baseType: 'Gold Amulet', rarity: 'rare', class: 'Amulets', mods: [{ statId: 'explicit.spell', values: [3] }] }
const draft = { status: 'securable', name: '', baseType: '', rarity: 'rare', filters: [{ group: 'type_filters', id: 'category', option: 'accessory.amulet' }] }

test('a category-only overlay draft does not restore the copied item base', () => {
  const selected = marketItemFromDraft(item, draft)
  assert.equal(selected.baseType, '')
  assert.equal(selected.name, '')
  assert.equal(selected.rarity, 'rare')
  assert.equal(selected.class, 'Amulets')
  assert.equal(selected.mods, item.mods)
  assert.equal(item.baseType, 'Gold Amulet')
})

test('disabled unique name and rarity remain empty in a transferred draft', () => {
  const unique = { ...item, rarity: 'unique', name: 'Astramentis' }
  const selected = marketItemFromDraft(unique, { ...draft, baseType: 'Gold Amulet', rarity: '' })
  assert.equal(selected.name, '')
  assert.equal(selected.baseType, 'Gold Amulet')
  assert.equal(selected.rarity, '')
})

test('an untouched draft leaves the snapshot identity available', () => {
  assert.equal(marketItemFromDraft(item), item)
  assert.equal(marketItemFromDraft(item, { status: '', name: '', baseType: '', rarity: '' }), item)
})

test('clearing or whitespace in the market box removes the base and unique name', () => {
  for (const query of ['', '   ']) {
    const selected = editMarketIdentity({ ...item, rarity: 'unique', name: 'Astramentis' }, query)
    assert.equal(selected.name, '')
    assert.equal(selected.baseType, '')
    assert.equal(selected.rarity, 'unique')
    assert.equal(selected.class, 'Amulets')
    assert.equal(selected.mods, item.mods)
  }
  assert.equal(item.baseType, 'Gold Amulet')
})

test('clearing survives a tab snapshot and a subsequent draft transfer', () => {
  const cleared = structuredClone(editMarketIdentity(item, ''))
  const selected = marketItemFromDraft(cleared, draft)
  assert.equal(selected.name, '')
  assert.equal(selected.baseType, '')
  assert.deepEqual(selected.mods, item.mods)
})
