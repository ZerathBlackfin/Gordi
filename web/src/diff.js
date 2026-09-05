import { t } from './i18n.svelte.js'

export function compare(before = {}, after = {}) {
  const keys = new Set([...Object.keys(before ?? {}), ...Object.keys(after ?? {})])
  const out = []
  for (const key of [...keys].sort()) {
    const a = (before?.[key] ?? []).join(' / ')
    const b = (after?.[key] ?? []).join(' / ')
    if (a === b) continue
    out.push({ key, before: a, after: b, kind: !a ? 'added' : !b ? 'removed' : 'replaced' })
  }
  return out
}

export function split(diffs) {
  if (diffs.length === 0) return { shared: [], own: [] }

  const counts = new Map()
  for (const list of diffs) {
    for (const d of list) {
      const signature = `${d.key}\u0000${d.before}\u0000${d.after}`
      counts.set(signature, (counts.get(signature) ?? 0) + 1)
    }
  }

  const everywhere = (d) =>
    counts.get(`${d.key}\u0000${d.before}\u0000${d.after}`) === diffs.length

  const shared = diffs[0].filter(everywhere)
  const own = diffs.map((list) => list.filter((d) => !everywhere(d)))
  return { shared, own }
}

export function labelFor(key) {
  const name = t(`tag.${key}`)
  return name === `tag.${key}` ? key : name
}

const PAGES = {
  MUSICBRAINZ_ALBUMID: 'release',
  MUSICBRAINZ_ALBUMARTISTID: 'artist',
  MUSICBRAINZ_ARTISTID: 'artist',
  MUSICBRAINZ_RELEASEGROUPID: 'release-group',
  MUSICBRAINZ_TRACKID: 'recording',
  MUSICBRAINZ_RELEASETRACKID: 'track',
  MUSICBRAINZ_WORKID: 'work',
}

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i

export function linkFor(key, value) {
  const page = PAGES[key]
  if (!page || !UUID.test(value ?? '')) return null
  return `https://musicbrainz.org/${page}/${value}`
}
