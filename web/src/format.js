import { t, currentLanguage } from './i18n.svelte.js'

export function duration(ms) {
  if (!ms) return '—'
  const s = Math.round(ms / 1000)
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const rest = s % 60
  if (h > 0) return `${h} h ${String(m).padStart(2, '0')}`
  return `${m}:${String(rest).padStart(2, '0')}`
}

export function size(bytes) {
  if (!bytes) return '—'
  const mb = bytes / (1024 * 1024)
  if (mb >= 1024) return `${decimal(mb / 1024)} ${t('unit.gb')}`
  if (mb >= 1) return `${mb.toFixed(0)} ${t('unit.mb')}`
  return `${Math.round(bytes / 1024)} ${t('unit.kb')}`
}

function decimal(n) {
  return new Intl.NumberFormat(currentLanguage.value, {
    minimumFractionDigits: 1,
    maximumFractionDigits: 1,
  }).format(n)
}

export function day(iso) {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return '—'
  return new Intl.DateTimeFormat(currentLanguage.value, {
    day: 'numeric',
    month: 'short',
    year: 'numeric',
  }).format(d)
}

export function durationGap(a, b) {
  if (!a || !b) return null
  return Math.round((b - a) / 1000)
}
