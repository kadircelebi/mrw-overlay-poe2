// Search-as-you-type matching shared by the market's lists: every typed word
// must appear, in any order. Hyphens count as spaces and apostrophes are
// ignored, so "time lost" finds "Time-Lost Ruby" and "kaoms" "Kaom's Heart".
function fold(text: string) {
  return text.toLocaleLowerCase().replace(/['’]/g, '').replace(/-/g, ' ')
}

export function wordMatcher(query: string): (text: string) => boolean {
  const words = fold(query).split(/\s+/).filter(Boolean)
  return (text) => { const folded = fold(text); return words.every((word) => folded.includes(word)) }
}
