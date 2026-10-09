import { useEffect, useState } from 'react';
import {
  ArrowRight,
  Check,
  Film,
  Layers3,
  LockKeyhole,
  Pause,
  Play,
  Plus,
  RotateCcw,
  Save,
  Trash2,
} from 'lucide-react';
import { api } from '../lib/api';
import { defaults, referencedAssets } from '../lib/generation';
import type {
  Asset,
  GenerationOptions,
  Job,
  LongVideo,
  LongVideoSettings,
  SegmentRequest,
} from '../types';
import { Composer } from './Composer';

const labels: Record<string, string> = {
  paused: '已暂停',
  waiting: '等待补充',
  queued: '排队中',
  running: '生成中',
  completed: '已完成',
  failed: '失败待处理',
  cancelled: '已取消',
};
const seconds = (value: number) => Number(value.toFixed(3));
function previewTimings(segments: SegmentRequest[], settings: LongVideoSettings) {
  let previous = 0;
  return segments.map((seg, index) => {
    const trim =
      index === 0
        ? 0
        : [124, 107, 90, 73, 56, 39, 22, 5, 1].find(
            (n) => n <= settings.context_frames && n <= previous,
          ) || 0;
    const needed = Math.ceil(seg.duration * settings.fps - 1e-8) + trim;
    previous = 5 + 17 * Math.ceil(Math.max(0, needed - 5) / 17);
    return { input: previous / settings.fps, effective: (previous - trim) / settings.fps, trim };
  });
}

type Props = {
  projectId?: string;
  resumeID?: string;
  settings: LongVideoSettings;
  onSettingsRefresh: (settings: LongVideoSettings) => void;
  onNewVideo: () => void;
  assets: Asset[];
  jobs: Job[];
  initialPrompt: string;
  initialOptions: GenerationOptions;
  onRefresh: () => Promise<void>;
  onUpload: () => void;
  uploading: boolean;
};

export function LongVideoEditor(props: Props) {
  const { settings, assets, jobs, onUpload, uploading } = props;
  const draftKey = `localdream-long-draft:${props.projectId || 'personal'}`;
  const blank = (config: LongVideoSettings = settings): SegmentRequest => ({
    ...defaults,
    mode: 'multi_reference_video',
    duration: config.default_seconds,
    prompt: '',
    asset_ids: [],
  });
  const [saved, setSaved] = useState<LongVideo>();
  const [videos, setVideos] = useState<LongVideo[]>([]);
  const [title, setTitle] = useState('我的连续镜头');
  const [segments, setSegments] = useState<SegmentRequest[]>(() =>
    Array.from({ length: settings.default_segments }, (_, i) => ({
      ...blank(),
      aspect_ratio: props.initialOptions.aspect_ratio,
      resolution: props.initialOptions.resolution,
      reference_video_fps: props.initialOptions.reference_video_fps,
      prompt: i === 0 ? props.initialPrompt : '',
    })),
  );
  const [active, setActive] = useState(0);
  const [dirty, setDirty] = useState(false);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState('');
  const [error, setError] = useState('');
  const [loaded, setLoaded] = useState(false);
  useEffect(() => {
    let disposed = false;
    if (props.resumeID)
      api
        .longVideo(props.resumeID)
        .then((value) => {
          if (!disposed) apply(value);
        })
        .catch((e) => {
          if (!disposed) setError(e.message);
        });
    return () => {
      disposed = true;
    };
  }, [props.resumeID]);
  const live = videos.find((v) => v.id === saved?.id) || saved;
  const config = saved?.settings || settings;
  const timings = previewTimings(segments, config);
  const current = segments[active];
  const currentSegment = live?.segments[active];
  const job = jobs.find((g) => g.id === currentSegment?.job_id);
  const locked = !!currentSegment?.job_id;
  const total = timings.reduce((sum, t) => sum + t.effective, 0);

  useEffect(() => {
    try {
      const raw =
        sessionStorage.getItem(draftKey) ||
        (!props.projectId ? sessionStorage.getItem('localdream-long-draft') : null);
      if (raw && !props.resumeID) {
        const draft = JSON.parse(raw);
        if (Array.isArray(draft.segments) && draft.segments.length) {
          // Preserve existing editing sessions, but make their snapshot visible.
          // Explicit "New video" below always discards that association.
          setSaved(draft.saved);
          setTitle(draft.title);
          setSegments(
            draft.segments.map((seg: SegmentRequest) => ({
              ...defaults,
              ...seg,
              reference_video_fps: seg.reference_video_fps ?? 0,
            })),
          );
          setDirty(draft.dirty);
        }
      }
    } catch {
      /* The server remains authoritative; a missing local draft is harmless. */
    }
    setLoaded(true);
  }, []);
  useEffect(() => {
    if (loaded) sessionStorage.setItem(draftKey, JSON.stringify({ saved, title, segments, dirty }));
  }, [loaded, saved, title, segments, dirty]);
  useEffect(() => {
    let disposed = false;
    let timer: ReturnType<typeof setTimeout>;
    const poll = async () => {
      try {
        const list = await api.longVideos();
        if (!disposed)
          setVideos(list.filter((v) => (v.project_id || '') === (props.projectId || '')));
      } catch (e) {
        if (!disposed) setError(e instanceof Error ? e.message : '无法读取长视频记录');
      }
      if (!disposed) timer = setTimeout(poll, 3000);
    };
    void poll();
    return () => {
      disposed = true;
      clearTimeout(timer);
    };
  }, []);
  // Polls update state only when no local edits would be overwritten.
  useEffect(() => {
    if (live && !dirty && live.revision !== saved?.revision) apply(live);
  }, [live?.revision, dirty]);

  function apply(value: LongVideo) {
    if ((value.project_id || '') !== (props.projectId || '')) {
      setError('此视频属于其他创作空间，请从对应项目进入。');
      return;
    }
    setSaved(value);
    setTitle(value.title);
    setSegments(
      value.segments.map((s) => ({
        ...defaults,
        ...s.request,
        reference_video_fps: s.request.reference_video_fps ?? 0,
      })),
    );
    setVideos((list) => [value, ...list.filter((v) => v.id !== value.id)]);
    setDirty(false);
    setActive((i) => Math.min(i, value.segments.length - 1));
  }
  function change(patch: Partial<SegmentRequest>) {
    if (busy) return;
    setSegments((list) =>
      list.map((seg, i) => {
        const format =
          !saved && active === 0
            ? {
                aspect_ratio: patch.aspect_ratio || seg.aspect_ratio,
                resolution: patch.resolution || seg.resolution,
              }
            : {};
        return i === active
          ? { ...seg, ...patch, ...format, mode: 'multi_reference_video', use_agent: false }
          : { ...seg, ...format };
      }),
    );
    setDirty(true);
    setMessage('');
  }
  function resize(count: number) {
    if (!Number.isInteger(count) || count < 1 || count > 100) return;
    const submitted = live?.segments.filter((s) => !!s.job_id).length || 0;
    if (count < submitted) {
      setError('已提交的片段不能移除。');
      return;
    }
    setSegments((list) =>
      Array.from(
        { length: count },
        (_, i) =>
          list[i] || {
            ...blank(),
            aspect_ratio: list[0].aspect_ratio,
            resolution: list[0].resolution,
          },
      ),
    );
    setActive((i) => Math.min(i, count - 1));
    setDirty(true);
  }
  async function save(start: boolean) {
    if (busy) return;
    setBusy(true);
    setError('');
    setMessage('');
    try {
      const body = {
        project_id: saved?.project_id || props.projectId || '',
        title,
        aspect_ratio: segments[0].aspect_ratio,
        resolution: segments[0].resolution,
        revision: live?.revision,
        start,
        segments: segments.map((seg, i) => ({
          ...seg,
          asset_ids: live?.segments[i]?.job_id
            ? seg.asset_ids
            : referencedAssets(seg.prompt, assets).map((a) => a.id),
        })),
      };
      const value = await api.saveSequence(saved?.id, body);
      apply(value);
      setMessage(
        start
          ? '计划已提交。逐段生成，遇到空白片段自动等待。'
          : '计划已保存，后续分段暂不启动；当前任务不受影响。',
      );
      await props.onRefresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : '保存失败');
    } finally {
      setBusy(false);
    }
  }
  async function action(kind: 'pause' | 'start' | 'retry') {
    if (!saved || busy) return;
    setBusy(true);
    setError('');
    try {
      const value = await api.sequenceAction(saved.id, kind);
      if (!dirty) apply(value);
      else {
        setSaved((old) => old && { ...old, revision: value.revision });
        setVideos((list) => [value, ...list.filter((v) => v.id !== value.id)]);
      }
      setMessage(
        kind === 'pause' ? '已暂停后续分段；当前正在执行的片段会继续完成。' : '已恢复分段计划。',
      );
      await props.onRefresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : '操作失败');
    } finally {
      setBusy(false);
    }
  }
  async function submitSegment() {
    if (busy) return;
    if (!saved) {
      await save(true);
      return;
    }
    const request = {
      ...current,
      asset_ids: referencedAssets(current.prompt, assets).map((a) => a.id),
      project_id: saved.project_id || props.projectId || '',
    };
    setBusy(true);
    setError('');
    setMessage('');
    try {
      const previousID = saved.id;
      const value = await api.regenerateSegment(previousID, active + 1, request);
      apply(value);
      setMessage(
        value.id === previousID
          ? `片段 ${active + 1} 已保存并重新提交。`
          : `已创建新的连续视频版本，从片段 ${active + 1} 继续生成；原版本保留。`,
      );
      await props.onRefresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : '保存并提交失败');
    } finally {
      setBusy(false);
    }
  }
  async function open(id: string) {
    if (dirty && !window.confirm('尚有未保存的分段编辑，确定切换？')) return;
    setError('');
    setMessage('');
    const value = videos.find((v) => v.id === id);
    if (value) {
      setActive(0);
      apply(value);
      return;
    }
    setBusy(true);
    try {
      // Read at the actual new-video boundary, including changes saved in
      // another tab. Do not reuse an older parent prop or stored plan snapshot.
      const latest = await api.longVideoSettings();
      props.onSettingsRefresh(latest);
      props.onNewVideo();
      setSaved(undefined);
      setActive(0);
      setTitle('我的连续镜头');
      const fresh = Array.from({ length: latest.default_segments }, () => blank(latest));
      setSegments(fresh);
      setDirty(false);
      sessionStorage.setItem(
        draftKey,
        JSON.stringify({ title: '我的连续镜头', segments: fresh, dirty: false }),
      );
      setMessage(
        `已新建视频，采用最新全局设置：画面 ${latest.context_frames} 帧，音频 ${latest.audio_context_frames} 帧。`,
      );
    } catch (e) {
      setError(e instanceof Error ? e.message : '读取最新长视频设置失败');
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="long-studio" aria-label="长视频工作台">
      <header className="long-heading">
        <div>
          <div className="eyebrow">
            <Layers3 size={14} /> CONTINUOUS STORY
          </div>
          <h2>让故事，继续发生。</h2>
          <p>逐段创作，自然续接。先完成一个镜头，再决定下一个。</p>
        </div>
        <button className="outline-button" onClick={() => void open('')} disabled={busy}>
          <Plus size={15} /> 新建视频
        </button>
      </header>
      <div className="sequence-topline">
        <label className="sequence-name">
          视频名称
          <input
            value={title}
            disabled={busy}
            onChange={(e) => {
              setTitle(e.target.value);
              setDirty(true);
            }}
            aria-label="长视频名称"
          />
        </label>
        <label>
          继续已有视频
          <select
            aria-label="继续已有长视频"
            value={saved?.id || ''}
            disabled={busy}
            onChange={(e) => void open(e.target.value)}
          >
            <option value="">新的视频</option>
            {videos.map((v) => (
              <option key={v.id} value={v.id}>
                {v.title} · {v.completed}/{v.segments.length} 段
              </option>
            ))}
          </select>
        </label>
        <div className="sequence-state">
          <span className="mint-dot" />
          {dirty ? '有未保存编辑' : live ? labels[live.status] : '手动分段'}
          {saved && (
            <small>
              {live?.completed || 0} / {segments.length} 段完成
            </small>
          )}
        </div>
      </div>
      {error && (
        <div className="notice error" role="alert">
          <span>{error}</span>
          {live && (
            <button
              className="text-button"
              onClick={() => {
                if (!dirty || window.confirm('重新载入将替换尚未保存的编辑，继续？')) {
                  apply(live);
                  setError('');
                }
              }}
            >
              重新载入
            </button>
          )}
        </div>
      )}
      {message && (
        <div className="notice success" role="status">
          <Check size={16} />
          {message}
        </div>
      )}
      {live?.error && (
        <p className="form-error" role="alert">
          {live.error}
        </p>
      )}
      <div className="sequence-body">
        <aside className="shot-sidebar">
          <div className="shot-sidebar-heading">
            <span>分段计划</span>
            <label>
              <input
                aria-label="连续生成段数"
                type="number"
                min={1}
                max={100}
                value={segments.length}
                disabled={busy}
                onChange={(e) => resize(Number(e.target.value))}
              />{' '}
              段
            </label>
          </div>
          <div className="shot-list">
            {segments.map((seg, i) => {
              const clipJob = jobs.find((g) => g.id === live?.segments[i]?.job_id);
              return (
                <button
                  key={i}
                  className={`shot-card ${active === i ? 'selected' : ''}`}
                  aria-pressed={active === i}
                  onClick={() => setActive(i)}
                >
                  <span className="shot-number">
                    {clipJob?.status === 'completed' ? (
                      <Check size={16} />
                    ) : (
                      String(i + 1).padStart(2, '0')
                    )}
                  </span>
                  <span className="shot-info">
                    <strong>
                      片段 {i + 1}
                      <small>{seconds(timings[i].effective)}s</small>
                    </strong>
                    <span>
                      {clipJob
                        ? labels[clipJob.status]
                        : seg.prompt.trim()
                          ? '已填写 · 待生成'
                          : '留白 · 稍后补充'}
                    </span>
                    <span>
                      输入 {seconds(timings[i].input)}s · 裁 {timings[i].trim} 帧
                    </span>
                  </span>
                </button>
              );
            })}
          </div>
          <button
            className="add-shot"
            disabled={busy}
            onClick={() => {
              resize(segments.length + 1);
              setActive(segments.length);
            }}
          >
            <Plus size={15} /> 添加下一段
          </button>
          <div className="sequence-total">
            <span>预计有效总时长</span>
            <strong>
              {seconds(total)}
              <small> 秒</small>
            </strong>
            <span>独立输出每段 · 暂不合并</span>
          </div>
        </aside>
        <div className="shot-editor">
          <div className="shot-editor-heading">
            <div>
              <span className="eyebrow">SHOT {String(active + 1).padStart(2, '0')}</span>
              <h3>{active === 0 ? '故事从这里开始' : '接着上一段，写下新的画面'}</h3>
            </div>
            {!locked && active === segments.length - 1 && active > 0 && (
              <button
                className="icon-button"
                aria-label="移除最后一段"
                disabled={busy}
                onClick={() => resize(segments.length - 1)}
              >
                <Trash2 size={16} />
              </button>
            )}
          </div>
          <div className="continuity-note">
            <LockKeyhole size={14} />
            <span>
              {active === 0 && !saved
                ? '在第一段设定画幅，整条视频保持一致。'
                : '比例与分辨率保持一致；其余生成参数可逐段调整。'}{' '}
              {saved ? '当前计划快照' : '最新全局设置'}：{config.fps} fps · 画面{' '}
              {config.context_frames} 帧 · 音频 {config.audio_context_frames} 帧
            </span>
          </div>
          {job?.output_url && (
            <video className="segment-preview" src={job.output_url} controls preload="metadata" />
          )}
          {job?.error && (
            <p className="form-error" role="alert">
              {job.error}
            </p>
          )}
          {job && (
            <div className="clip-progress">
              <span>
                {labels[job.status]}
                {job.progress_stage ? ` · ${job.progress_stage}` : ''}
              </span>
              <span>
                {job.progress_known
                  ? `${job.progress}%`
                  : job.status === 'completed'
                    ? '100%'
                    : '等待进度'}
              </span>
              <progress
                max={100}
                value={
                  job.status === 'completed' ? 100 : job.progress_known ? job.progress : undefined
                }
              />
            </div>
          )}
          <Composer
            key={`${saved?.id || 'new'}-${active}`}
            longVideo
            formatLocked={!!saved || active > 0}
            prompt={current.prompt}
            onPrompt={(prompt) => change({ prompt })}
            options={current}
            onOptions={(value) => change(value)}
            assets={assets}
            busy={busy}
            readOnly={false}
            agentBusy={false}
            uploading={uploading}
            onUpload={onUpload}
            onGenerate={() => void (locked ? submitSegment() : save(true))}
            onInterpret={() => {}}
            generateLabel={locked ? '保存并重新生成' : saved ? '保存并继续生成' : '开始连续生成'}
          />
          <div className="timing-breakdown">
            <div>
              <span>想要的有效时长</span>
              <strong>{current.duration}s</strong>
            </div>
            <ArrowRight size={14} />
            <div>
              <span>工作流输入</span>
              <strong>{seconds(timings[active].input)}s</strong>
            </div>
            <ArrowRight size={14} />
            <div>
              <span>实际预计保留</span>
              <strong>{seconds(timings[active].effective)}s</strong>
            </div>
          </div>
          <p className="timing-note">
            {active === 0 &&
              '首段没有上一段，不补重叠帧；修改重叠设置只会改变第 2 段及后续段的输入时长。'}
            自动补偿 {timings[active].trim} 帧重叠，再按 H3 的 17k＋5 帧网格取整。无需手动加时长。
            {timings[active].input > config.max_input_seconds &&
              ' 当前超过建议分段时长；手动模式允许继续，请留意显存。'}
          </p>
          {locked && (
            <p className="timing-note">
              此片段已提交，比例和分辨率保持锁定；修改提示词、素材、时长或其他参数后，点击“保存并重新生成”创建新版本。原版本会保留。
            </p>
          )}
        </div>
      </div>
      <footer className="sequence-footer">
        <span>
          <Film size={16} /> 只填第一段也能开始；遇到空白片段时等待补充。
        </span>
        <div>
          {saved && !live?.paused && live?.status !== 'completed' && (
            <button className="text-button" disabled={busy} onClick={() => void action('pause')}>
              <Pause size={15} /> 暂停后续
            </button>
          )}
          {live?.status === 'failed' && (
            <button className="outline-button" disabled={busy} onClick={() => void action('retry')}>
              <RotateCcw size={15} /> 重试失败段
            </button>
          )}
          <button className="outline-button" disabled={busy} onClick={() => void save(false)}>
            <Save size={15} /> 保存计划
          </button>
          <button
            className="primary-button"
            disabled={
              busy ||
              uploading ||
              (locked
                ? !current.prompt.trim()
                : !segments.some((seg, i) => !live?.segments[i]?.job_id && seg.prompt.trim()))
            }
            onClick={() => void (locked ? submitSegment() : save(true))}
          >
            <Play size={15} />
            {locked ? '保存并重新生成' : saved ? '保存并继续' : '开始生成'}
          </button>
        </div>
      </footer>
    </section>
  );
}
