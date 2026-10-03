// The craft library: saved crafts the player can go back to. Each entry
// keeps the whole craft (item, history, session start) plus a summary for the
// list. The app writes the list to a file (see craft_library.go); this module
// only shapes it.

export const libraryLimit = 100;
// Crafts saved automatically when a new one starts; the oldest beyond this
// many give way, saved-by-hand ones never do.
export const autoLimit = 20;

export const validEntry = e => Boolean(e && typeof e.id === 'string' && e.state && e.state.item &&
  Array.isArray(e.state.item.mods) && Array.isArray(e.state.history));

export function parseLibrary(text) {
  try {
    const list = JSON.parse(text);
    return Array.isArray(list) ? list.filter(validEntry) : [];
  } catch { return []; }
}

// entryFor turns the current craft into a library entry. summary carries what
// the list shows that the page knows best (base name, cost).
export function entryFor({ item, history, sessionStart }, { name, auto = false, baseName = '', costEx = null, now = new Date() }) {
  return {
    id: `${now.getTime().toString(36)}-${Math.random().toString(36).slice(2, 8)}`,
    name: String(name || '').trim().slice(0, 80),
    auto,
    saved_at: now.toISOString(),
    page: item.base,
    baseName,
    rarity: item.rarity,
    mods: item.mods.length,
    steps: history.length,
    cost_ex: costEx,
    corrupted: Boolean(item.corrupted), sanctified: Boolean(item.sanctified),
    state: { format: 2, item: structuredClone(item), history: structuredClone(history), sessionStart },
  };
}

// sameCraft: the craft is already in the library as it is now.
export const sameCraft = (entry, { item, history }) =>
  JSON.stringify(entry.state.item) === JSON.stringify(item) && JSON.stringify(entry.state.history) === JSON.stringify(history);

export function addEntry(list, entry) {
  let next = [entry, ...list.filter(e => e.id !== entry.id)];
  const autos = next.filter(e => e.auto);
  if (autos.length > autoLimit) {
    const drop = new Set(autos.slice(autoLimit).map(e => e.id));
    next = next.filter(e => !drop.has(e.id));
  }
  return next.slice(0, libraryLimit);
}

export const removeEntry = (list, id) => list.filter(e => e.id !== id);
export const renameEntry = (list, id, name) => list.map(e => e.id === id ? { ...e, name: String(name).trim().slice(0, 80), auto: false } : e);
