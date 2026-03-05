/**
 * Version composable — provides both frontend and backend API versions,
 * plus update-check state from GET /api/v1/version/check.
 *
 * Frontend version is injected at build time from package.json.
 * API version is fetched from GET /api/v1/version on mount.
 * Update availability is checked on mount and every 6 hours.
 */
export function useVersion() {
  const config = useRuntimeConfig();
  const uiVersion = (config.public.appVersion as string) || '0.0.0';
  const uiBuildDate = (config.public.appBuildDate as string) || '';

  const apiVersion = ref('');
  const apiCommit = ref('');
  const apiBuildDate = ref('');

  const updateAvailable = ref(false);
  const latestVersion = ref('');
  const releaseUrl = ref('');

  let checkInterval: ReturnType<typeof setInterval> | null = null;

  async function fetchApiVersion() {
    try {
      const api = useApi();
      const data = (await api('/api/v1/version')) as {
        version?: string;
        commit?: string;
        buildDate?: string;
      };
      apiVersion.value = data.version || '';
      apiCommit.value = data.commit || '';
      apiBuildDate.value = data.buildDate || '';
    } catch (e) {
      // API version endpoint may not exist yet — graceful degradation
      console.warn('[useVersion] fetchApiVersion failed:', e);
      apiVersion.value = '';
    }
  }

  async function checkForUpdates() {
    try {
      const api = useApi();
      const data = (await api('/api/v1/version/check')) as {
        current?: string;
        latest?: string;
        updateAvailable?: boolean;
        releaseUrl?: string;
      };
      updateAvailable.value = data.updateAvailable || false;
      latestVersion.value = data.latest || '';
      releaseUrl.value = data.releaseUrl || '';
    } catch (e) {
      console.warn('[useVersion] checkForUpdates failed:', e);
    }
  }

  onMounted(() => {
    fetchApiVersion();
    checkForUpdates();
    checkInterval = setInterval(checkForUpdates, 6 * 60 * 60 * 1000); // 6 hours
  });

  onBeforeUnmount(() => {
    if (checkInterval) {
      clearInterval(checkInterval);
      checkInterval = null;
    }
  });

  return {
    uiVersion,
    uiBuildDate,
    apiVersion: readonly(apiVersion),
    apiCommit: readonly(apiCommit),
    apiBuildDate: readonly(apiBuildDate),
    updateAvailable: readonly(updateAvailable),
    latestVersion: readonly(latestVersion),
    releaseUrl: readonly(releaseUrl),
    checkForUpdates,
  };
}
