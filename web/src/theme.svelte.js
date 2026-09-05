
const KEY = 'gordi-theme'

export const OPTIONS = [{ value: 'system' }, { value: 'light' }, { value: 'dark' }]

const VALID = OPTIONS.map((o) => o.value)

function read() {
  try {
    const v = localStorage.getItem(KEY)
    return VALID.includes(v) ? v : 'system'
  } catch {
    return 'system'
  }
}

let choice = $state(read())

function apply(v) {
  const root = document.documentElement
  if (v === 'system') root.removeAttribute('data-theme')
  else root.setAttribute('data-theme', v)
}

apply(choice)

export const theme = {
  get choice() {
    return choice
  },
}

export function choose(value) {
  if (!VALID.includes(value)) return
  choice = value
  apply(choice)
  try {
    localStorage.setItem(KEY, choice)
  } catch {
  }
}
