
let path = $state(window.location.pathname)

export const route = {
  get path() {
    return path
  },
}

export function navigate(to) {
  if (to === path) return
  history.pushState({}, '', to)
  path = to
}

window.addEventListener('popstate', () => {
  path = window.location.pathname
})
