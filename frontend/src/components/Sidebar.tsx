import { ArrowUpRight, Clock3, FolderOpen, Settings2, Sparkles, Video } from 'lucide-react';
import type { View } from '../types';
import { SidebarProjects, type ProjectNavigation } from './SidebarProjects';

type Props = ProjectNavigation & {
  view: View;
  onNavigate: (view: View) => void;
  onSettings: () => void;
  activeCount: number;
};
export function Sidebar(props: Props) {
  const { view, onNavigate, onSettings, activeCount, projectId } = props;
  return (
    <aside className="sidebar">
      <button className="brand" onClick={() => onNavigate('create')} aria-label="Local Dream 首页">
        <span className="brand-symbol">
          <Sparkles size={23} />
        </span>
        <span>
          Local Dream<small>让灵感，自由生长</small>
        </span>
      </button>
      <div className="nav-caption">工作空间</div>
      <nav aria-label="主导航">
        {(
          [
            { id: 'create', label: '创作空间', icon: Sparkles },
            { id: 'assets', label: '素材库', icon: FolderOpen },
            { id: 'history', label: '生成记录', icon: Clock3 },
          ] as const
        ).map(({ id, label, icon: Icon }) => (
          <button
            key={id}
            className={`nav-item ${view === id && !projectId ? 'active' : ''}`}
            aria-label={label}
            aria-current={view === id && !projectId ? 'page' : undefined}
            onClick={() => onNavigate(id)}
          >
            <Icon size={19} />
            <span>{label}</span>
            {id === 'history' && activeCount > 0 && <span className="count">{activeCount}</span>}
          </button>
        ))}
      </nav>
      <SidebarProjects {...props} />
      <div className="sidebar-note">
        <span className="mini-label">MADE FOR YOUR IDEAS</span>
        <div className="orbit-art" aria-hidden="true">
          <i />
          <i />
          <Sparkles size={24} />
        </div>
        <strong>想象，不止于此。</strong>
        <p>
          从一个念头，到一段故事。
          <br />
          下一帧，由你定义。
        </p>
        <button onClick={() => onNavigate('create')}>
          开始创作 <ArrowUpRight size={15} />
        </button>
      </div>
      <div className="sidebar-bottom">
        <button className="nav-item" aria-label="模型与设置" onClick={onSettings}>
          <Settings2 size={18} />
          <span>模型与设置</span>
        </button>
        <div className="local-profile">
          <span className="avatar">L</span>
          <span>
            我的本地工作室
            <small>
              <Video size={11} /> ComfyUI 驱动
            </small>
          </span>
          <span className="local-tag">LOCAL</span>
        </div>
      </div>
    </aside>
  );
}
