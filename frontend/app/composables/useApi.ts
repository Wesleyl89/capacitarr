import { ofetch } from 'ofetch'

export const useApi = () => {
  const config = useRuntimeConfig()
  const authenticated = useCookie('authenticated')

  const apiFetch = ofetch.create({
    baseURL: config.public.apiBaseUrl as string,
    // The HttpOnly 'jwt' cookie is sent automatically by the browser
    // for same-origin requests — no need to set Authorization header manually.
    credentials: 'include',
    onResponseError({ response }) {
      if (response.status === 401) {
        const router = useRouter()
        authenticated.value = null
        router.push('/login')
      }
    }
  })

  return apiFetch
}
