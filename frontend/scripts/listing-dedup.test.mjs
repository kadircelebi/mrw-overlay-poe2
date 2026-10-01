import { test } from 'node:test'
import assert from 'node:assert/strict'
import { uniqueListings } from '../src/lib/listingDedup.ts'

const listing = (id, patch = {}) => ({ id, account: 'Seller#1234', amount: 19, currency: 'chaos', listed: '2026-09-22', hideoutToken: 'token-'+id,
  item: { name: 'Visions of Paradise', baseType: 'Irradiated Tablet', itemLevel: 81, icon: 'image-'+id, properties: [{name:'Uses',value:'1'}], mods: [{type:'unique',description:'Completing Map adds Irradiation'}] }, ...patch })

test('same seller, currency, price and item appears once across fetched pages', () => {
  const first = listing('a')
  assert.deepEqual(uniqueListings([first, listing('b',{listed:'2026-09-23'}), listing('c')]), [first])
})
test('different sellers, currency or prices remain visible', () => {
  assert.equal(uniqueListings([listing('a'),listing('b',{account:'Other#1234'}),listing('c',{amount:23}),listing('d',{currency:'divine'})]).length,4)
})
test('different item levels, uses, rolls and corruption remain visible', () => {
  const original=listing('a')
  const items=[{...original.item,itemLevel:82},{...original.item,properties:[{name:'Uses',value:'2'}]},
    {...original.item,mods:[{type:'unique',description:'Different roll'}]},{...original.item,corrupted:true}]
  assert.equal(uniqueListings([original,...items.map((item,i)=>listing(String(i),{item}))]).length,5)
})
test('mod and property ordering does not create duplicates', () => {
  const original=listing('a');original.item.mods.push({type:'implicit',description:'Adds Irradiated to a Map'});original.item.properties.push({name:'Quality',value:'20'})
  const reordered=listing('b',{item:{...original.item,mods:[...original.item.mods].reverse(),properties:[...original.item.properties].reverse()}})
  assert.equal(uniqueListings([original,reordered]).length,1)
})
test('duplicate modifier counts and gem attributes remain significant', () => {
  const original=listing('a')
  assert.equal(uniqueListings([original,listing('b',{item:{...original.item,mods:[...original.item.mods,...original.item.mods]}}),listing('c',{item:{...original.item,gemLevel:20}})]).length,3)
})
test('missing seller identity is never merged', () => {
  assert.equal(uniqueListings([listing('a',{account:''}),listing('b',{account:''})]).length,2)
})
