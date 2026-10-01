// Currency icons come from the trade site at run time instead of being shipped
// in the package: the art belongs to Grinding Gear Games. The trade data names
// each icon by a generated URL whose first segment after /gen/image/ is
// base64url JSON holding the game's art path ("2DItems/Currency/Omens/…"),
// the same path the craft data uses ("Art/2DItems/Currency/Omens/….webp").

export function artKey(icon) {
  return String(icon || '').replace(/^Art\//, '').replace(/\.[a-z0-9]+$/i, '');
}

function decodeArtPath(url) {
  const at = url.indexOf('/gen/image/');
  if (at < 0) return '';
  const segment = url.slice(at + '/gen/image/'.length).split('/')[0].replace(/-/g, '+').replace(/_/g, '/');
  try {
    const parts = JSON.parse(atob(segment + '='.repeat((4 - segment.length % 4) % 4)));
    const file = parts.find((part) => part && typeof part.f === 'string');
    return file ? file.f : '';
  } catch {
    return '';
  }
}

// iconIndex maps art paths to image URLs. When a path cannot be decoded the
// file name still identifies the icon, but only if no two icons share it.
export function iconIndex(entries) {
  const byPath = new Map();
  const byName = new Map();
  for (const entry of entries || []) {
    const url = typeof entry?.image === 'string' ? entry.image : '';
    if (!url.startsWith('https://')) continue;
    const path = decodeArtPath(url);
    if (path && !byPath.has(path)) byPath.set(path, url);
    const name = url.split('/').at(-1).replace(/\.[a-z0-9]+$/i, '');
    const known = byName.get(name);
    if (known === undefined) byName.set(name, url);
    else if (known !== url) byName.set(name, null);
  }
  return { byPath, byName };
}

export function iconFor(index, icon) {
  const key = artKey(icon);
  if (!key || !index) return '';
  return index.byPath.get(key) || index.byName.get(key.split('/').at(-1)) || '';
}
