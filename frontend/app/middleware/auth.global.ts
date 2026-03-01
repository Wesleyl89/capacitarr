/**
 * Global auth middleware.
 * Redirects unauthenticated users to /login for all pages except /login itself.
 */
export default defineNuxtRouteMiddleware((to) => {
  const token = useCookie('jwt')

  if (to.path !== '/login' && !token.value) {
    return navigateTo('/login')
  }

  // If already authenticated and trying to visit login, redirect to dashboard
  if (to.path === '/login' && token.value) {
    return navigateTo('/')
  }
})
