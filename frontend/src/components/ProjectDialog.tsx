import { useEffect, useRef, useState } from 'react';
import { FolderOpen, X } from 'lucide-react';
import type { Project } from '../types';

export function ProjectDialog({
  project,
  onClose,
  onSave,
}: {
  project?: Project;
  onClose: () => void;
  onSave: (name: string, description: string) => Promise<void>;
}) {
  const ref = useRef<HTMLDialogElement>(null);
  const [name, setName] = useState(project?.name || '');
  const [description, setDescription] = useState(project?.description || '');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  useEffect(() => {
    ref.current?.showModal();
    ref.current?.querySelector<HTMLInputElement>('input')?.focus();
  }, []);
  return (
    <dialog
      ref={ref}
      className="settings-dialog project-dialog"
      aria-labelledby="project-dialog-title"
      onCancel={(e) => {
        e.preventDefault();
        if (!busy) onClose();
      }}
    >
      <div className="dialog-heading">
        <span className="empty-symbol">
          <FolderOpen size={24} />
        </span>
        <button className="icon-button" aria-label="关闭项目编辑" disabled={busy} onClick={onClose}>
          <X size={19} />
        </button>
      </div>
      <h2 id="project-dialog-title">{project ? '编辑项目' : '新建项目'}</h2>
      <p className="muted">将素材与创作收在一起，让每个故事有自己的空间。</p>
      <form
        onSubmit={async (e) => {
          e.preventDefault();
          if (!name.trim() || busy) return;
          setBusy(true);
          setError('');
          try {
            await onSave(name.trim(), description.trim());
            onClose();
          } catch (e) {
            setError(e instanceof Error ? e.message : '保存失败');
          } finally {
            setBusy(false);
          }
        }}
      >
        <fieldset className="settings-fields" disabled={busy}>
          <label>
            项目名称
            <input
              autoFocus
              required
              maxLength={80}
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="例如：提弗洛斯的一天"
            />
          </label>
          <label>
            项目描述 <small>可选</small>
            <textarea
              rows={3}
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="记录这个项目的主题或创作方向"
            />
          </label>
        </fieldset>
        {error && (
          <p className="form-error" role="alert">
            {error}
          </p>
        )}
        <div className="dialog-footer">
          <button type="button" className="text-button" disabled={busy} onClick={onClose}>
            取消
          </button>
          <button className="primary-button" disabled={busy || !name.trim()}>
            {busy ? '保存中…' : project ? '保存修改' : '创建项目'}
          </button>
        </div>
      </form>
    </dialog>
  );
}
