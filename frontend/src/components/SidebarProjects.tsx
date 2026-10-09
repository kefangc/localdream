import { useEffect, useState } from 'react';
import {
  Archive,
  ArchiveRestore,
  ChevronDown,
  ChevronRight,
  Folder,
  FolderOpen,
  Pencil,
  Plus,
  SquarePen,
  Trash2,
} from 'lucide-react';
import type { Project } from '../types';
import { ActionMenu } from './ActionMenu';

export type ProjectNavigation = {
  projects: Project[];
  projectId: string;
  locked: boolean;
  onOpenProject: (id: string) => void;
  onCreateProject: () => void;
  onEditProject: (project: Project) => void;
  onArchiveProject: (project: Project) => void;
  onRemoveProject: (project: Project) => void;
};
export function SidebarProjects(props: ProjectNavigation) {
  const [expanded, setExpanded] = useState(() => window.innerWidth > 760);
  useEffect(() => {
    const media = window.matchMedia('(max-width: 760px)');
    const change = () => setExpanded(!media.matches);
    media.addEventListener('change', change);
    return () => media.removeEventListener('change', change);
  }, []);
  const [archives, setArchives] = useState(false);
  const active = props.projects.filter((p) => !p.archived);
  const archived = props.projects.filter((p) => p.archived);
  function row(p: Project) {
    return (
      <div className={`sidebar-project ${props.projectId === p.id ? 'selected' : ''}`} key={p.id}>
        <button
          type="button"
          className="project-link"
          aria-label={`打开项目 ${p.name}`}
          aria-current={props.projectId === p.id ? 'page' : undefined}
          disabled={props.locked || p.archived}
          title={p.name}
          onClick={() => {
            props.onOpenProject(p.id);
            if (window.innerWidth <= 760) setExpanded(false);
          }}
        >
          <Folder size={17} />
          <span>{p.name}</span>
        </button>
        <div className="project-row-actions">
          <ActionMenu
            label={`项目 ${p.name} 的更多操作`}
            disabled={props.locked}
            items={[
              {
                label: '编辑项目',
                icon: <Pencil size={15} />,
                onSelect: () => props.onEditProject(p),
              },
              {
                label: p.archived ? '恢复项目' : '归档项目',
                icon: p.archived ? <ArchiveRestore size={15} /> : <Archive size={15} />,
                onSelect: () => props.onArchiveProject(p),
              },
              {
                label: '移除项目',
                icon: <Trash2 size={15} />,
                danger: true,
                onSelect: () => props.onRemoveProject(p),
              },
            ]}
          />
          <button
            type="button"
            className="icon-button project-enter"
            aria-label={`进入 ${p.name} 的创作空间`}
            title="进入项目创作空间"
            disabled={props.locked || p.archived}
            onClick={() => {
              props.onOpenProject(p.id);
              if (window.innerWidth <= 760) setExpanded(false);
            }}
          >
            <SquarePen size={17} />
          </button>
        </div>
      </div>
    );
  }
  return (
    <section className={`sidebar-projects ${expanded ? 'expanded' : ''}`} aria-label="项目">
      <div className="projects-heading">
        <button
          type="button"
          aria-expanded={expanded}
          aria-controls="sidebar-project-list"
          onClick={() => setExpanded(!expanded)}
          title="展开或收起项目"
        >
          <FolderOpen size={17} />
          <span>项目</span>
          {expanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
        </button>
        <button
          className="icon-button"
          aria-label="新建项目"
          title="新建项目"
          disabled={props.locked}
          onClick={props.onCreateProject}
        >
          <Plus size={16} />
        </button>
      </div>
      {expanded && (
        <div id="sidebar-project-list" className="sidebar-project-list">
          <div className="mobile-project-heading">
            <strong>项目</strong>
            <button
              className="icon-button"
              aria-label="新建项目"
              disabled={props.locked}
              onClick={props.onCreateProject}
            >
              <Plus size={16} />
            </button>
            <button className="text-button" onClick={() => setExpanded(false)}>
              收起
            </button>
          </div>
          {active.map(row)}
          {!active.length && (
            <button
              className="project-empty"
              onClick={props.onCreateProject}
              disabled={props.locked}
            >
              <Plus size={14} />
              创建你的第一个项目
            </button>
          )}
          {archived.length > 0 && (
            <>
              <button
                className="archived-project-toggle"
                aria-expanded={archives}
                onClick={() => setArchives(!archives)}
              >
                <Archive size={13} />
                已归档 <span>{archived.length}</span>
                {archives ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
              </button>
              {archives && archived.map(row)}
            </>
          )}
        </div>
      )}
    </section>
  );
}
