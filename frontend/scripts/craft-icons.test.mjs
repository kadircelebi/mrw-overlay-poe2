import test from 'node:test';
import assert from 'node:assert/strict';
import { artKey, iconIndex, iconFor } from '../public/craft/icons.mjs';

// The trade site's image URL: /gen/image/<base64url JSON>/<hash>/<File>.png
function tradeURL(path, file = path.split('/').at(-1)) {
  const segment = Buffer.from(JSON.stringify([25, 14, { f: path, scale: 1, realm: 'poe2' }])).toString('base64url');
  return `https://web.poecdn.com/gen/image/${segment}/0123456789/${file}.png`;
}

test('craft art paths match the trade site icons by the encoded game path', () => {
  const index = iconIndex([
    { id: 'aug', text: 'Orb of Augmentation', image: tradeURL('2DItems/Currency/CurrencyAddModToMagic') },
    { id: 'omen', text: 'Omen of Light', image: tradeURL('2DItems/Currency/Omens/VoodooOmens1Yellow') },
  ]);
  assert.equal(artKey('Art/2DItems/Currency/Omens/VoodooOmens1Yellow.webp'), '2DItems/Currency/Omens/VoodooOmens1Yellow');
  assert.equal(iconFor(index, 'Art/2DItems/Currency/CurrencyAddModToMagic.webp'), tradeURL('2DItems/Currency/CurrencyAddModToMagic'));
  assert.equal(iconFor(index, 'Art/2DItems/Currency/Omens/VoodooOmens1Yellow.webp'), tradeURL('2DItems/Currency/Omens/VoodooOmens1Yellow'));
});

test('an icon the trade site does not list stays empty so the fallback shows', () => {
  const index = iconIndex([{ id: 'aug', image: tradeURL('2DItems/Currency/CurrencyAddModToMagic') }]);
  assert.equal(iconFor(index, 'Art/2DItems/Currency/Omens/VoodooOmens1Green.webp'), '');
  assert.equal(iconFor(null, 'Art/2DItems/Currency/CurrencyAddModToMagic.webp'), '');
  assert.equal(iconFor(index, ''), '');
});

test('the file name is a fallback only when it names a single icon', () => {
  const index = iconIndex([
    { id: 'a', image: 'https://web.poecdn.com/image/Art/2DItems/Currency/Plain.png' },
    { id: 'b', image: 'https://web.poecdn.com/image/x/Twin.png' },
    { id: 'c', image: 'https://web.poecdn.com/image/y/Twin.png' },
  ]);
  assert.equal(iconFor(index, 'Art/2DItems/Currency/Plain.webp'), 'https://web.poecdn.com/image/Art/2DItems/Currency/Plain.png');
  assert.equal(iconFor(index, 'Art/2DItems/Currency/Twin.webp'), '');
});

test('only https images are used', () => {
  const index = iconIndex([{ id: 'x', image: 'javascript:alert(1)//Evil.png' }, { id: 'y', image: '/gen/image/rel/Rel.png' }]);
  assert.equal(iconFor(index, 'Art/Evil.webp'), '');
  assert.equal(iconFor(index, 'Art/Rel.webp'), '');
});
