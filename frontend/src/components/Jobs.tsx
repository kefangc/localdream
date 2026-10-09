import { useEffect, useState } from 'react';
import { ArrowRight, Clock3, RefreshCw, Video } from 'lucide-react';
import { isActiveJob, modeLabel } from '../lib/generation';
import type { Job, Project } from '../types';
import { JobActions } from './JobActions';

type Props = {
  jobs: Job[];
  compact?: boolean;
  onRefresh: () => void;
  onReuse: (job: Job) => void;
  onContinue?: (id: string) => void;
  onArchive?: (job: Job) => Promise<void>;
  onDelete?: (job: Job) => Promise<void>;
  projects?: Project[];
  onMove?: (job: Job, projectId: string) => Promise<void>;
  onCancel: (job: Job) => Promise<void>;
  onViewAll?: () => void;
  locked: boolean;
};
const statusLabels: Record<string, string> = {
  queued: '排队中',
  running: '生成中',
  completed: '已完成',
  failed: '生成失败',
  cancelled: '已取消',
};

function formatElapsed(milliseconds: number) {
  const seconds = Math.max(0, Math.floor(milliseconds / 1000));
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const rest = seconds % 60;
  return `${hours ? `${hours} 小时 ` : ''}${minutes || hours ? `${minutes} 分 ` : ''}${rest} 秒`;
}

function elapsedLabel(job: Job, now: number) {
  if (job.status === 'queued') return null;
  if (!isActiveJob(job.status)) {
    return typeof job.elapsed_ms === 'number' && Number.isFinite(job.elapsed_ms)
      ? `总耗时 ${formatElapsed(job.elapsed_ms)}`
      : null;
  }
  const start = Date.parse(job.started_at ?? '');
  const end = job.finished_at ? Date.parse(job.finished_at) : now;
  return Number.isFinite(start) && Number.isFinite(end)
    ? `已用时 ${formatElapsed(end - start)}`
    : null;
}

function promptSummary(prompt: string) {
  const section = prompt.match(
    /(?:^|\n)summary:\s*([\s\S]*?)(?=\n\s*(?:retention_analysis|detailed_description|timeline|overall_soundscape):|$)/i,
  );
  return (section?.[1] || prompt)
    .replace(/\[[^\]]*\]/g, '')
    .replace(/\s+/g, ' ')
    .trim();
}

function createdLabel(value?: string) {
  if (!value || !Number.isFinite(Date.parse(value))) return '';
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }).format(new Date(value));
}

export function Jobs({
  jobs,
  compact,
  onRefresh,
  onReuse,
  onCancel,
  onViewAll,
  locked,
  onContinue,
  onArchive,
  onDelete,
  projects = [],
  onMove,
}: Props) {
  const [filter, setFilter] = useState('all');
  const [now, setNow] = useState(Date.now);
  const hasActive = jobs.some((job) => job.status === 'running');
  useEffect(() => {
    if (!hasActive) return;
    setNow(Date.now());
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, [hasActive]);
  const filtered = jobs.filter(
    (job) =>
      !job.deleted &&
      (filter === 'archived'
        ? !!job.archived
        : !job.archived &&
          (filter === 'all' ||
            (filter === 'active' ? isActiveJob(job.status) : job.status === filter))),
  );
  const visible = compact
    ? jobs.filter((job) => !job.archived && !job.deleted).slice(0, 3)
    : filtered;
  return (
    <section className="jobs-section">
      <div className="section-heading">
        <div>
          {!compact && <span className="eyebrow">YOUR CREATIVE JOURNEY</span>}
          {compact ? (
            <h2>
              最近创作 <span className="heading-count">{jobs.length}</span>
            </h2>
          ) : (
            <h1>生成记录</h1>
          )}
        </div>
        <div className="inline-actions">
          <button className="icon-button" aria-label="刷新生成记录" onClick={onRefresh}>
            <RefreshCw size={16} />
          </button>
          {compact && (
            <button className="text-button" onClick={onViewAll}>
              查看全部
              <ArrowRight size={14} />
            </button>
          )}
        </div>
      </div>
      {!compact && (
        <>
          <p className="muted">追踪每次生成，重用你喜欢的镜头。记录保存在本地数据库中。</p>
          <div className="filter-tabs history-filters">
            {[
              ['all', '全部'],
              ['active', '进行中'],
              ['completed', '已完成'],
              ['failed', '失败'],
              ['cancelled', '已取消'],
              ['archived', '已归档'],
            ].map(([id, label]) => (
              <button
                key={id}
                aria-pressed={filter === id}
                className={filter === id ? 'selected' : ''}
                onClick={() => setFilter(id)}
              >
                {label}
              </button>
            ))}
          </div>
        </>
      )}
      {!visible.length ? (
        <div className={`empty-state ${compact ? 'compact-empty' : ''}`}>
          <span className="empty-symbol">
            <Clock3 size={23} />
          </span>
          <div>
            <h3>{jobs.length ? '这里暂时没有匹配的记录' : '你的第一段故事，即将开始'}</h3>
            <p>
              {jobs.length
                ? '切换筛选，查看其他生成记录。'
                : '提交创作后，生成进度与视频作品会出现在这里。'}
            </p>
          </div>
          {compact && (
            <span className="empty-decoration" aria-hidden="true">
              ✳
            </span>
          )}
        </div>
      ) : (
        <div className="job-grid">
          {visible.map((job) => (
            <article className="job-card record-card" key={job.id}>
              <div className="job-preview">
                {job.output_url ? (
                  <video src={job.output_url} controls preload="metadata" />
                ) : (
                  <div className="job-placeholder">
                    <Video size={28} />
                    <span>{statusLabels[job.status] ?? job.status}</span>
                  </div>
                )}
                <span className={`job-status ${job.status}`}>
                  {statusLabels[job.status] ?? job.status}
                </span>
              </div>
              <div className="job-info">
                <div className="job-title">
                  <strong>
                    {job.continuity ? `长视频 · 片段 ${job.continuity.index}` : modeLabel(job.mode)}
                  </strong>
                  <time dateTime={job.created_at}>{createdLabel(job.created_at)}</time>
                </div>
                <p className="job-prompt" title={promptSummary(job.prompt)}>
                  {promptSummary(job.prompt) || '没有填写画面描述'}
                </p>
                <p className="job-specs record-specs">
                  {job.continuity
                    ? Number(job.continuity.timing.effective_seconds.toFixed(3))
                    : job.duration}
                  s · {job.resolution} · {job.aspect_ratio} · {job.steps} 步
                  {job.lora ? ' · LoRA' : ''}
                </p>
                <div className="record-context">
                  <span>
                    {projects.find((p) => p.id === job.project_id)?.name ||
                      (job.project_id ? '已归档或移除的项目' : '个人创作')}
                  </span>
                  {job.archived && <span>已归档</span>}
                  {job.superseded && <span>历史版本</span>}
                </div>
                {elapsedLabel(job, now) && (
                  <p className="job-specs" title="从 ComfyUI 开始执行到结束，不包含排队等待时间">
                    {elapsedLabel(job, now)}
                  </p>
                )}
                {isActiveJob(job.status) && (
                  <p className="job-specs" role="status">
                    {job.status === 'queued' ? '等待执行' : job.progress_stage || '等待节点进度'}
                    {job.status === 'running' && job.progress_known
                      ? ` · ${job.progress_value ?? 0}/${job.progress_max ?? 0}${job.progress_stage === '采样' ? ' 步' : ''}`
                      : ''}
                    {job.status === 'running' && !job.progress_connected
                      ? job.progress_known
                        ? ' · 保留上次进度，正在重连'
                        : ' · 正在连接实时进度'
                      : ''}
                  </p>
                )}
                {isActiveJob(job.status) && (
                  <div className="progress-row">
                    <progress
                      max={100}
                      value={
                        job.progress_known
                          ? Math.max(0, Math.min(100, job.progress || 0))
                          : undefined
                      }
                      aria-label="当前节点进度，不代表整个任务完成度"
                    />
                    <span>{job.progress_known ? `${Math.round(job.progress || 0)}%` : '—'}</span>
                  </div>
                )}
                {job.error && <p className="job-error">{job.error}</p>}
                <JobActions
                  job={job}
                  projects={projects}
                  locked={locked}
                  onReuse={onReuse}
                  onContinue={onContinue}
                  onArchive={onArchive}
                  onDelete={onDelete}
                  onMove={onMove}
                  onCancel={onCancel}
                />
              </div>
            </article>
          ))}
        </div>
      )}
    </section>
  );
}
