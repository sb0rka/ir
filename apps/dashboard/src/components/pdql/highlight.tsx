import type { ReactNode } from 'react'

export function highlightMatch(text: string, query: string): ReactNode {
  const needle = query.trim()
  if (!needle) return text
  const index = text.toLowerCase().indexOf(needle.toLowerCase())
  if (index < 0) return text
  return (
    <>
      {text.slice(0, index)}
      <mark className="rounded-sm bg-proposed/40 text-fg">{text.slice(index, index + needle.length)}</mark>
      {text.slice(index + needle.length)}
    </>
  )
}
