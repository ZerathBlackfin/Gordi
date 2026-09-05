
import en from './locales/en.json'
import fr from './locales/fr.json'

const CATALOG = { en, fr }

let language = $state('en')

export const currentLanguage = {
  get value() {
    return language
  },
}

export function setLanguage(v) {
  if (CATALOG[v]) language = v
}

export function t(key, values = {}) {
  let text = CATALOG[language]?.[key] ?? CATALOG.en[key] ?? key

  if (text.includes('|')) {
    const [singular, plural] = text.split('|')
    const n = Math.abs(values.n ?? 0)
    text = (language === 'fr' ? n > 1 : n !== 1) ? plural : singular
  }

  return text.replace(/\{(\w+)\}/g, (token, name) =>
    values[name] === undefined ? token : String(values[name]),
  )
}
