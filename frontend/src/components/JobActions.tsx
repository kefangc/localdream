import { useEffect, useRef, useState } from 'react';
import {
  Archive,
  ArchiveRestore,
  ArrowUpRight,
  Copy,
  Download,
  FileText,
  Folder,
  Trash2,
  X,
} from 'lucide-react';
import type { Job, Project } from '../types';
import { isActiveJob, modeLabel } from '../lib/generation';
import { ActionMenu, type MenuItem } from './ActionMenu';

type Props = {
  job: Job;
  projects: Project[];
  locked: boolean;
  onReuse: (job: Job) => void;
  onContinue?: (id: string) => void;
  onArchive?: (job: Job) => Promise<void>;
  onDelete?: (job: Job) => Promise<void>;
  onMove?: (job: Job, projectId: string) => Promise<void>;
  onCancel: (job: Job) => Promise<void>;
};
function RecordDetails({ job, onClose }: { job: Job; onClose: () => void }) {
  const dialog = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    dialog.current?.showModal();
  }, []);
  return (
    <dialog
      className="settings-dialog record-details"
      ref={dialog}
      aria-labelledby="record-details-title"
      onCancel={(e) => {
        e.preventDefault();
        onClose();
      }}
    >
      <div className="dialog-heading">
        <h2 id="record-details-title">生成详情</h2>
        <button className="icon-button" aria-label="关闭生成详情" onClick={onClose}>
          <X size={18} />
        </button>
      </div>
      <p className="muted">
        {modeLabel(job.mode)} · {job.duration}s · {job.resolution} · {job.aspect_ratio} ·{' '}
        {job.steps} 步
      </p>
      {job.mode === 'multi_reference_video' && (
        <p className="muted">
          参考视频 FPS：{job.reference_video_fps ? `${job.reference_video_fps}fps` : '源帧率'}
          {' · '}成片 24fps
        </p>
      )}
      <div className="record-identifier">
        <span>任务 ID</span>
        <code>{job.id}</code>
      </div>
      <h3>完整提示词</h3>
      <pre className="record-prompt-full">{job.prompt || '没有提示词'}</pre>
    </dialog>
  );
}
export function JobActions(props: Props) {
  const { job, locked, projects, onReuse, onContinue, onArchive, onDelete, onMove, onCancel } =
    props;
  const [busy, setBusy] = useState(false);
  const [details, setDetails] = useState(false);
  const [error, setError] = useState('');
  async function perform(action: () => Promise<void>) {
    setBusy(true);
    setError('');
    try {
      await action();
    } catch (e) {
      setError(e instanceof Error ? e.message : '操作失败');
    } finally {
      setBusy(false);
    }
  }
  const items: MenuItem[] = [
    { label: '查看参数与提示词', icon: <FileText size={15} />, onSelect: () => setDetails(true) },
  ];
  if (job.continuity)
    items.push({
      label: '重用描述',
      icon: <Copy size={15} />,
      onSelect: () => onReuse(job),
      disabled: locked,
    });
  if (!isActiveJob(job.status)) {
    if (onArchive)
      items.push({
        label: job.archived ? '取消归档' : '归档记录',
        icon: job.archived ? <ArchiveRestore size={15} /> : <Archive size={15} />,
        onSelect: () => void perform(() => onArchive(job)),
      });
    if (onDelete)
      items.push({
        label: '删除记录',
        icon: <Trash2 size={15} />,
        danger: true,
        onSelect: () => void perform(() => onDelete(job)),
      });
  } else
    items.push({
      label: '取消任务',
      icon: <X size={15} />,
      danger: true,
      onSelect: () => void perform(() => onCancel(job)),
    });
  return (
    <>
      <div className="record-actions">
        <button
          className="record-primary"
          disabled={locked || busy}
          onClick={() =>
            job.continuity && onContinue ? onContinue(job.continuity.sequence_id) : onReuse(job)
          }
        >
          {job.continuity ? <ArrowUpRight size={15} /> : <Copy size={14} />}
          {job.continuity ? '继续视频' : '重用描述'}
        </button>
        <div className="record-secondary">
          {job.output_url && (
            <a
              className="icon-button"
              href={job.output_url}
              download
              aria-label={`下载视频 ${job.id}`}
              title="下载视频"
            >
              <Download size={16} />
            </a>
          )}
          <ActionMenu
            label={`生成记录 ${job.id} 的更多操作`}
            items={items}
            disabled={locked || busy}
          >
            {onMove && (
              <label className="record-move">
                <span>
                  <Folder size={13} />
                  移动到项目
                </span>
                <select
                  aria-label={`移动记录 ${job.id}`}
                  value={job.project_id || ''}
                  disabled={busy}
                  onChange={(e) => void perform(() => onMove(job, e.target.value))}
                >
                  <option value="">不归属项目</option>
                  {projects.map((p) => (
                    <option value={p.id} key={p.id}>
                      {p.name}
                    </option>
                  ))}
                </select>
              </label>
            )}
          </ActionMenu>
        </div>
      </div>
      {error && (
        <p className="form-error" role="alert">
          {error}
        </p>
      )}
      {details && <RecordDetails job={job} onClose={() => setDetails(false)} />}
    </>
  );
}
