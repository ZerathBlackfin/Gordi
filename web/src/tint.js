
const cache = new Map()

export async function tintFor(url) {
  if (cache.has(url)) return cache.get(url)
  const tint = await extract(url).catch(() => null)
  cache.set(url, tint)
  return tint
}

function extract(url) {
  return new Promise((resolve, reject) => {
    const img = new Image()
    img.crossOrigin = 'anonymous'
    img.onerror = reject
    img.onload = () => {
      const c = document.createElement('canvas')
      c.width = c.height = 12
      const ctx = c.getContext('2d', { willReadFrequently: true })
      ctx.drawImage(img, 0, 0, 12, 12)
      const { data } = ctx.getImageData(0, 0, 12, 12)

      let best = null
      let bestScore = -1
      for (let i = 0; i < data.length; i += 4) {
        const [r, g, b] = [data[i], data[i + 1], data[i + 2]]
        const max = Math.max(r, g, b)
        const min = Math.min(r, g, b)
        const lightness = (0.2126 * r + 0.7152 * g + 0.0722 * b) / 255
        if (lightness < 0.12 || lightness > 0.88) continue

        const saturation = max === 0 ? 0 : (max - min) / max
        const score = saturation * (1 - Math.abs(lightness - 0.5))
        if (score > bestScore) {
          bestScore = score
          best = [r, g, b]
        }
      }

      if (!best || bestScore < 0.02) return resolve(null)
      resolve(readable(best))
    }
    img.src = url
  })
}

// Avoids 0.18-0.23 luminance: neither white nor dark ink reaches 4.5:1 there.
function readable([r, g, b]) {
  const [h, s, l] = toHSL(r / 255, g / 255, b / 255)
  const sat = Math.min(0.72, Math.max(0.38, s))
  let lum = Math.min(0.54, Math.max(0.34, l))

  let out = toRGB(h, sat, lum)
  while (lum > 0.06 && perceivedLuminance(out) > 0.18 && perceivedLuminance(out) < 0.23) {
    lum -= 0.01
    out = toRGB(h, sat, lum)
  }

  const [r2, g2, b2] = out
  return `rgb(${Math.round(r2 * 255)} ${Math.round(g2 * 255)} ${Math.round(b2 * 255)})`
}

export function inkOn(color) {
  const channels = String(color ?? '').match(/\d+(?:\.\d+)?/g)
  if (!channels || channels.length < 3) return LIGHT_INK
  const l = perceivedLuminance(channels.slice(0, 3).map((c) => Number(c) / 255))
  return l > 0.205 ? DARK_INK : LIGHT_INK
}

const LIGHT_INK = '#ffffff'
const DARK_INK = '#16191b'

function perceivedLuminance([r, g, b]) {
  const lin = (c) => (c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4)
  return 0.2126 * lin(r) + 0.7152 * lin(g) + 0.0722 * lin(b)
}

function toHSL(r, g, b) {
  const max = Math.max(r, g, b)
  const min = Math.min(r, g, b)
  const l = (max + min) / 2
  if (max === min) return [0, 0, l]

  const d = max - min
  const s = l > 0.5 ? d / (2 - max - min) : d / (max + min)
  let h
  if (max === r) h = ((g - b) / d + (g < b ? 6 : 0)) / 6
  else if (max === g) h = ((b - r) / d + 2) / 6
  else h = ((r - g) / d + 4) / 6
  return [h, s, l]
}

function toRGB(h, s, l) {
  if (s === 0) return [l, l, l]
  const q = l < 0.5 ? l * (1 + s) : l + s - l * s
  const p = 2 * l - q
  return [channel(p, q, h + 1 / 3), channel(p, q, h), channel(p, q, h - 1 / 3)]
}

function channel(p, q, h) {
  if (h < 0) h += 1
  if (h > 1) h -= 1
  if (h < 1 / 6) return p + (q - p) * 6 * h
  if (h < 1 / 2) return q
  if (h < 2 / 3) return p + (q - p) * (2 / 3 - h) * 6
  return p
}
