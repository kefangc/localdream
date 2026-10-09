import type {
  Asset,
  GenerationOptions,
  Intent,
  Job,
  Provider,
  LongVideo,
  LongVideoSettings,
  SequenceInput,
  SegmentRequest,
  Project,
} from '../types';

// One HTTP boundary owns parsing and errors. Views only handle typed data or Error.
export async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const timeout = new AbortController();
  const timer = setTimeout(() => timeout.abort(), 60000);
  try {
    const response = await fetch(path, {
      ...init,
      signal: init.signal ? AbortSignal.any([init.signal, timeout.signal]) : timeout.signal,
    });
    const raw = await response.text();
    let data;
    try {
      data = raw ? JSON.parse(raw) : null;
    } catch {
      throw new Error('服务返回了无法识别的数据，请检查本地服务是否已启动。');
    }
    if (!response.ok) throw new Error(data?.error || `请求失败 (${response.status})`);
    return data as T;
  } catch (error) {
    if (error instanceof TypeError) throw new Error('无法连接本地服务，请检查连接后重试。');
    if (error instanceof DOMException && error.name === 'AbortError')
      throw new Error('请求已取消或超时，请稍后重试。');
    throw error;
  } finally {
    clearTimeout(timer);
  }
}
const json = (body: unknown): RequestInit => ({
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify(body),
});
const list = <T>(value: T[] | null): T[] => (Array.isArray(value) ? value : []);
export const api = {
  longVideos: () => request<LongVideo[]>('/api/long-videos'),
  longVideo: (id: string) => request<LongVideo>(`/api/long-videos/${encodeURIComponent(id)}`),
  saveSequence: (id: string | undefined, body: SequenceInput) =>
    request<LongVideo>(id ? `/api/long-videos/${encodeURIComponent(id)}` : '/api/long-videos', {
      ...json(body),
      method: id ? 'PUT' : 'POST',
    }),
  sequenceAction: (id: string, action: 'start' | 'pause' | 'retry') =>
    request<LongVideo>(`/api/long-videos/${encodeURIComponent(id)}/${action}`, json({})),
  longVideoSettings: () => request<LongVideoSettings>('/api/config/long-video'),
  saveLongVideoSettings: (value: LongVideoSettings) =>
    request<LongVideoSettings>('/api/config/long-video', json(value)),
  assets: async (signal?: AbortSignal) =>
    list(await request<Asset[] | null>('/api/assets', { signal })),
  jobs: async (signal?: AbortSignal) =>
    list(await request<Job[] | null>('/api/generations', { signal })),
  upload: (file: File, projectId = '') => {
    const body = new FormData();
    body.append('file', file);
    body.append('project_id', projectId);
    return request<Asset>('/api/assets', { method: 'POST', body });
  },
  removeAsset: (id: string) =>
    request<void>(`/api/assets/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  projects: async () => list(await request<Project[] | null>('/api/projects')),
  createProject: (name: string, description = '') =>
    request<Project>('/api/projects', json({ name, description })),
  updateProject: (id: string, name: string, description: string) =>
    request<Project>(`/api/projects/${encodeURIComponent(id)}`, {
      ...json({ name, description }),
      method: 'PUT',
    }),
  archiveProject: (id: string, archived: boolean) =>
    request<Project>(
      `/api/projects/${encodeURIComponent(id)}/${archived ? 'archive' : 'restore'}`,
      json({}),
    ),
  removeProject: (id: string) =>
    request<Project>(`/api/projects/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  projectAssets: (projectId: string, assetId: string) =>
    projectId
      ? request(
          `/api/projects/${encodeURIComponent(projectId)}/assets/${encodeURIComponent(assetId)}`,
          json({}),
        )
      : request(`/api/assets/${encodeURIComponent(assetId)}/project`, json({ project_id: '' })),
  archive: (id: string) =>
    request<Job>(`/api/generations/${encodeURIComponent(id)}/archive`, json({})),
  restore: (id: string) =>
    request<Job>(`/api/generations/${encodeURIComponent(id)}/restore`, json({})),
  deleteJob: (id: string) =>
    request<Job>(`/api/generations/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  moveJob: (id: string, project_id: string) =>
    request<Job>(`/api/generations/${encodeURIComponent(id)}/move`, json({ project_id })),
  regenerateSegment: (id: string, index: number, segment?: SegmentRequest) =>
    request<LongVideo>(
      `/api/long-videos/${encodeURIComponent(id)}/regenerate`,
      json({ index, segment }),
    ),
  generate: (options: GenerationOptions, prompt: string, asset_ids: string[]) =>
    request<Job>(
      '/api/generations',
      json({
        ...options,
        lora_strength: options.lora ? options.lora_strength : 0,
        prompt,
        asset_ids,
      }),
    ),
  cancel: (id: string) => request(`/api/generations/${encodeURIComponent(id)}/cancel`, json({})),
  interpret: (prompt: string, assets: Asset[]) =>
    request<Intent>('/api/agent/interpret', json({ prompt, assets })),
  provider: () => request<Provider>('/api/config/provider'),
  saveProvider: (provider: Provider) => request<Provider>('/api/config/provider', json(provider)),
  models: (provider: Provider) =>
    request<{ models: string[] }>('/api/providers/models', json(provider)),
};
