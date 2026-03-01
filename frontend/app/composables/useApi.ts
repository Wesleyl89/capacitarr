import { ofetch, type FetchOptions } from 'ofetch'

export const useApi = () => {
  const config = useRuntimeConfig()
  const token = useCookie('jwt')

  const apiFetch = ofetch.create({
    baseURL: config.public.apiBaseUrl as string,
    onRequest({ options }: { options: FetchOptions }) {
      if (token.value) {
        options.headers = new Headers(options.headers)
        options.headers.set('Authorization', `Bearer ${token.value}`)
      }
    },
    onResponseError({ response }) {
      if (response.status === 401) {
        const router = useRouter()
        token.value = null
        router.push('/login')
      }
    }
  })

  return apiFetch
}
