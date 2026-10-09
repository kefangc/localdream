import { useCallback, useEffect, useRef, useState } from 'react';
import { api } from '../lib/api';
import type { Asset, Job, Project } from '../types';

export function useLibrary() {
  const [assets, setAssets] = useState<Asset[]>([]);
  const [jobs, setJobs] = useState<Job[]>([]);
  const [projects, setProjects] = useState<Project[]>([]);
  const [connection, setConnection] = useState<'connecting' | 'online' | 'offline'>('connecting');
  const [loadError, setLoadError] = useState('');
  const revision = useRef(0);
  const refresh = useCallback(async (signal?: AbortSignal) => {
    const current = ++revision.current;
    try {
      const [nextAssets, nextJobs, nextProjects] = await Promise.all([
        api.assets(signal),
        api.jobs(signal),
        api.projects(),
      ]);
      // Ignore stale responses: an older poll must not overwrite a newer refresh.
      if (signal?.aborted || current !== revision.current) return;
      setAssets(nextAssets);
      setJobs(nextJobs);
      setProjects(nextProjects);
      setConnection('online');
      setLoadError('');
    } catch (error) {
      if (signal?.aborted || current !== revision.current) return;
      setConnection('offline');
      setLoadError(error instanceof Error ? error.message : '加载失败');
    }
  }, []);
  useEffect(() => {
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    // Recursive polling avoids overlapping requests when the backend is slow.
    const poll = async () => {
      await refresh(controller.signal);
      if (!controller.signal.aborted) timer = setTimeout(poll, 3000);
    };
    void poll();
    return () => {
      controller.abort();
      clearTimeout(timer);
    };
  }, [refresh]);
  return { assets, jobs, projects, connection, loadError, refresh, setProjects };
}
