import { useState } from 'react';
import { AudioLines, FolderOpen, Plus, Search, Trash2, Upload } from 'lucide-react';
import type { Asset, Project } from '../types';

type Props = {
  assets: Asset[];
  onUpload: () => void;
  uploading: boolean;
  onReference: (asset: Asset) => void;
  onDelete: (asset: Asset) => Promise<void>;
  locked: boolean;
  projects?: Project[];
  onAssignProject?: (asset: Asset, projectId: string) => Promise<void>;
};
export function AssetLibrary({
  assets,
  onUpload,
  uploading,
  onReference,
  onDelete,
  locked,
  projects = [],
  onAssignProject,
}: Props) {
  const [filter, setFilter] = useState('all');
  const [projectFilter, setProjectFilter] = useState('');
  const [search, setSearch] = useState('');
  const [deleting, setDeleting] = useState('');
  const visible = assets.filter(
    (asset) =>
      (!projectFilter || asset.project_id === projectFilter) &&
      (filter === 'all' || asset.type === filter) &&
      asset.name.toLowerCase().includes(search.toLowerCase()),
  );
  return (
    <section>
      <div className="page-heading">
        <div>
          <span className="eyebrow">YOUR CREATIVE MATERIALS</span>
          <h1>
            素材库<span className="heading-count">{visible.length}</span>
          </h1>
          <p>收集每一片灵感，在创作中用 @ 引用。</p>
        </div>
        <button className="primary-button" disabled={uploading} onClick={onUpload}>
          <Upload size={16} />
          {uploading ? '上传中…' : '上传素材'}
        </button>
      </div>
      <div className="library-toolbar">
        <label className="asset-project-filter">
          <span>项目</span>
          <select
            aria-label="素材项目筛选"
            value={projectFilter}
            onChange={(e) => setProjectFilter(e.target.value)}
          >
            <option value="">全部素材</option>
            {projects
              .filter((p) => !p.archived)
              .map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
          </select>
        </label>
        <div className="filter-tabs">
          {[
            ['all', '全部'],
            ['image', '图片'],
            ['video', '视频'],
            ['audio', '音频'],
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
        <label className="search-box">
          <Search size={16} />
          <input
            aria-label="搜索素材"
            placeholder="搜索素材名称"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
          />
        </label>
      </div>
      {!visible.length ? (
        <div className="empty-state">
          <span className="empty-symbol">
            <FolderOpen size={28} />
          </span>
          <h3>{assets.length ? '没有找到匹配的素材' : '为你的故事，添加第一份素材'}</h3>
          <p>
            {assets.length
              ? '试试其他关键词或素材类型。'
              : '支持图片、视频和音频。上传后可在不同创作中引用。'}
          </p>
          {!assets.length && (
            <button className="outline-button" disabled={uploading} onClick={onUpload}>
              <Plus size={16} />
              添加素材
            </button>
          )}
        </div>
      ) : (
        <div className="asset-grid">
          {visible.map((asset) => (
            <article className="asset-card" key={asset.id}>
              <div className="asset-preview">
                {asset.type === 'image' ? (
                  <img src={asset.url} alt={asset.name} loading="lazy" />
                ) : asset.type === 'video' ? (
                  <video src={asset.url} controls preload="metadata" />
                ) : (
                  <div className="audio-preview">
                    <AudioLines size={35} />
                    <audio src={asset.url} controls preload="metadata" />
                  </div>
                )}
              </div>
              <div className="asset-info">
                <strong title={asset.name}>{asset.name}</strong>
                <small>
                  {asset.type === 'image' ? '图片' : asset.type === 'video' ? '视频' : '音频'} ·{' '}
                  {(asset.size / 1024 / 1024).toFixed(2)} MB
                </small>
                <div className="asset-actions">
                  {onAssignProject && (
                    <select
                      aria-label={`绑定项目 ${asset.name}`}
                      value={asset.project_id || ''}
                      onChange={(e) => void onAssignProject(asset, e.target.value)}
                    >
                      <option value="">未绑定项目</option>
                      {projects.map((p) => (
                        <option key={p.id} value={p.id}>
                          {p.name}
                        </option>
                      ))}
                    </select>
                  )}
                  <button
                    className="text-button"
                    disabled={locked}
                    onClick={() => onReference(asset)}
                  >
                    <Plus size={14} />
                    引用到创作
                  </button>
                  <button
                    className="icon-button danger"
                    aria-label={`删除 ${asset.name}`}
                    disabled={deleting === asset.id}
                    onClick={async () => {
                      if (!window.confirm(`删除素材「${asset.name}」？此操作无法撤销。`)) return;
                      setDeleting(asset.id);
                      try {
                        await onDelete(asset);
                      } finally {
                        setDeleting('');
                      }
                    }}
                  >
                    <Trash2 size={15} />
                  </button>
                </div>
              </div>
            </article>
          ))}
        </div>
      )}
    </section>
  );
}
