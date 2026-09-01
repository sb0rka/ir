export function titlesForQueueIds(
  ids: string[],
  alerts: Record<string, { title: string }>,
  correlations: Record<string, { title: string }>,
): string[] {
  const seen = new Set<string>()
  const titles: string[] = []
  for (const id of ids) {
    const title = (correlations[id]?.title ?? alerts[id]?.title ?? '').trim()
    if (!title || seen.has(title)) continue
    seen.add(title)
    titles.push(title)
  }
  return titles
}
