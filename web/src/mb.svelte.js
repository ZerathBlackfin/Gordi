let calls = $state([])
let seq = 0

export const waiting = {
  get active() {
    return calls.length > 0
  },
  get since() {
    return calls[0]?.at ?? 0
  },
}

export function start(path) {
  const id = ++seq
  calls = [...calls, { id, path, at: Date.now() }]
  return id
}

export function finish(id) {
  calls = calls.filter((c) => c.id !== id)
}
