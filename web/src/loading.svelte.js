let onScreen = $state(0)

export const loader = {
  get onScreen() {
    return onScreen > 0
  },
}

export function show() {
  onScreen++
  return () => onScreen--
}
