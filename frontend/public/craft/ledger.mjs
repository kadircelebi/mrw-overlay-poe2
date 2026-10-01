export function usageEntry(id, name, prices) {
  const row = prices.currency.find(r => r.api_id === id) || prices.currency.find(r => r.name === name);
  const value = row?.value_ex;
  return {id,name,quantity:1,unit_ex:Number.isFinite(value) && value >= 0 ? value : null,
    league:prices.league,price_at:prices.generated_at,source:prices.origin};
}

export function summarize(history) {
  const rows = new Map(); let total = 0, unknown = 0;
  for (const entry of history) for (const usage of entry.usages || (entry.usage ? [entry.usage] : [])) {
    if (!usage) continue;
    const current = rows.get(usage.id) || {name:usage.name,quantity:0,cost_ex:0,unknown:0};
    current.quantity += usage.quantity;
    if (usage.unit_ex === null) { current.unknown += usage.quantity; unknown += usage.quantity; }
    else { const cost = usage.quantity*usage.unit_ex; current.cost_ex += cost; total += cost; }
    rows.set(usage.id,current);
  }
  return {rows:[...rows.values()],total,unknown};
}
