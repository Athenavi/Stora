// Magazine Theme API
const PLUGIN_SLUG = 'magazine';

export async function getThemeConfig(apiClient: any) {
    const res = await apiClient.get(`/themes/${PLUGIN_SLUG}/config`);
    return res.data;
}

export async function saveThemeConfig(apiClient: any, settings: any) {
    const res = await apiClient.put(`/themes/${PLUGIN_SLUG}/config`, {settings});
    return res.data;
}
