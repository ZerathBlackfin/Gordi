
let inFlight = $state(0)
let since = $state(0)

export const waiting = {
  get active() {
    return inFlight > 0
  },
  get since() {
    return since
  },
}

export function start() {
  if (inFlight === 0) since = Date.now()
  inFlight++
}

export function finish() {
  inFlight = Math.max(0, inFlight - 1)
}
