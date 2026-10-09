import { useEffect, useRef, useState } from 'react';
import { ArrowUpRight, CheckCircle2, CircleAlert, Sparkles, X } from 'lucide-react';
import { AssetLibrary } from './components/AssetLibrary';
import { Composer } from './components/Composer';
import { LongVideoEditor } from './components/LongVideoEditor';
import { Inspiration } from './components/Inspiration';
import { Jobs } from './components/Jobs';
import { SettingsDialog } from './components/SettingsDialog';
import { Sidebar } from './components/Sidebar';
import { CreationModes } from './components/CreationModes';
import { ProjectDialog } from './components/ProjectDialog';
import { useLibrary } from './hooks/useLibrary';
import { api } from './lib/api';
import {
  changeMode,
  defaults,
  isActiveJob,
  modes,
  ratios,
  referencedAssets,
  resolutions,
  validateGeneration,
} from './lib/generation';
import type {
  Asset,
  GenerationOptions,
  Job,
  Notice,
  View,
  LongVideoSettings,
  Project,
} from './types';

export function App() {
  const [view, setView] = useState<View>('create');
  const [prompt, setPrompt] = useState('');
  const [options, setOptions] = useState<GenerationOptions>(defaults);
  const [settings, setSettings] = useState(false);
  const [longSettings, setLongSettings] = useState<LongVideoSettings>();
  const [strategy, setStrategy] = useState<'auto' | 'normal' | 'long'>('auto');
  const [settingsError, setSettingsError] = useState('');
  const [resumeID, setResumeID] = useState('');
  useEffect(() => {
    if (settings) return;
    let disposed = false;
    api
      .longVideoSettings()
      .then((value) => {
        if (disposed) return;
        setLongSettings(value);
        setSettingsError('');
      })
      .catch((e) => {
        if (!disposed) setSettingsError(e.message);
      });
    return () => {
      disposed = true;
    };
  }, [settings]);
  useEffect(() => {
    if (longSettings) setStrategy(longSettings.strategy);
  }, [longSettings?.strategy]);
  const [notice, setNotice] = useState<Notice>(null);
  const [busy, setBusy] = useState(false);
  const [agentBusy, setAgentBusy] = useState(false);
  const [uploading, setUploading] = useState(false);
  const operation = useRef(false);
  const fileInput = useRef<HTMLInputElement>(null);
  const { assets, jobs, projects, connection, loadError, refresh } = useLibrary();
  const [projectId, setProjectId] = useState('');
  const [projectDialog, setProjectDialog] = useState<Project | 'new' | null>(null);
  const drafts = useRef<
    Record<
      string,
      { prompt: string; options: GenerationOptions; strategy: 'auto' | 'normal' | 'long' }
    >
  >({});
  const activeProject = projects.find((p) => p.id === projectId);
  const scopedAssets = assets.filter((asset) => !projectId || asset.project_id === projectId);
  const fail = (error: unknown) =>
    setNotice({
      kind: 'error',
      message: error instanceof Error ? error.message : '操作失败，请重试。',
    });
  const focusPrompt = () =>
    requestAnimationFrame(() => document.getElementById('creative-prompt')?.focus());
  function navigate(next: View) {
    setView(next);
    window.scrollTo({ top: 0, behavior: 'instant' });
  }
  function enterWorkspace(id: string, next: View = 'create') {
    if (operation.current || uploading) return;
    if (id !== projectId) {
      drafts.current[projectId] = { prompt, options, strategy };
      const draft = drafts.current[id];
      setPrompt(draft?.prompt || '');
      setOptions(draft?.options || defaults);
      setStrategy(draft?.strategy || longSettings?.strategy || 'auto');
      setProjectId(id);
    }
    setResumeID('');
    setNotice(null);
    navigate(next);
  }
  function chooseIdea(value: string) {
    if (operation.current) return;
    setPrompt(value);
    setOptions(changeMode(options, 'text_to_video'));
    focusPrompt();
  }
  async function upload(files: FileList | null) {
    if (!files?.length || uploading) return;
    setUploading(true);
    setNotice(null);
    let count = 0;
    try {
      for (const file of Array.from(files)) {
        await api.upload(file, projectId);
        count++;
      }
      setNotice({
        kind: 'success',
        message: `已上传 ${count} 份素材，使用 @ 引用后即可参与创作。`,
      });
    } catch (error) {
      fail(
        new Error(
          `${count ? `已上传 ${count} 份素材。` : ''}${error instanceof Error ? error.message : '上传失败'}`,
        ),
      );
    } finally {
      setUploading(false);
      if (fileInput.current) fileInput.current.value = '';
      await refresh();
    }
  }
  async function generate() {
    if (operation.current || uploading) return;
    const validation = validateGeneration(prompt, options, scopedAssets);
    if (validation) {
      fail(new Error(validation));
      return;
    }
    operation.current = true;
    setBusy(true);
    setNotice(null);
    try {
      await api.generate(
        { ...options, strategy, project_id: projectId },
        prompt,
        referencedAssets(prompt, scopedAssets).map((asset) => asset.id),
      );
      setNotice({ kind: 'success', message: '任务已提交，可在生成记录中查看进度。' });
      await refresh();
    } catch (error) {
      fail(error);
    } finally {
      // Always release the lock, including network failures, so retry remains possible.
      operation.current = false;
      setBusy(false);
    }
  }
  async function interpret() {
    if (operation.current || !prompt.trim()) return;
    operation.current = true;
    setAgentBusy(true);
    setNotice(null);
    try {
      const intent = await api.interpret(prompt, scopedAssets);
      const references = referencedAssets(prompt, scopedAssets);
      const description =
        intent.prompt &&
        [
          intent.prompt.subject,
          intent.prompt.motion,
          intent.prompt.camera,
          intent.prompt.environment,
          intent.prompt.constraints,
        ]
          .filter(Boolean)
          .join('；');
      if (description)
        setPrompt(`${references.map((asset) => `@${asset.name}`).join(' ')} ${description}`.trim());
      // Treat model output as untrusted: only adopt values the UI actually supports.
      setOptions((current) => ({
        ...current,
        ...(intent.mode && modes.some((mode) => mode.id === intent.mode)
          ? { mode: intent.mode, lora: '' }
          : {}),
        ...(intent.duration &&
        Number.isFinite(Number(intent.duration)) &&
        Number(intent.duration) >= 1 &&
        Number(intent.duration) <= 15
          ? { duration: Number(intent.duration) }
          : {}),
        ...(intent.aspect_ratio && ratios.includes(intent.aspect_ratio)
          ? { aspect_ratio: intent.aspect_ratio }
          : {}),
        ...(intent.resolution && resolutions.includes(intent.resolution)
          ? { resolution: intent.resolution }
          : {}),
      }));
      setNotice({ kind: 'success', message: '描述已完善。检查参数后，再点击生成视频。' });
    } catch (error) {
      fail(error);
    } finally {
      operation.current = false;
      setAgentBusy(false);
    }
  }
  function reference(asset: Asset) {
    if (operation.current) return;
    setPrompt((value) => `${value}${value && !value.endsWith(' ') ? ' ' : ''}@${asset.name} `);
    if (options.mode === 'text_to_video')
      setOptions(
        changeMode(options, asset.type === 'image' ? 'image_to_video' : 'multi_reference_video'),
      );
    navigate('create');
    focusPrompt();
  }
  function reuse(job: Job) {
    if (operation.current) return;
    setPrompt(job.prompt);
    setOptions({
      ...defaults,
      mode: job.mode,
      duration: job.duration,
      aspect_ratio: job.aspect_ratio,
      resolution: job.resolution,
      steps: job.steps,
      reference_video_fps: job.reference_video_fps ?? 0,
      lora: job.lora || '',
      lora_strength: job.lora_strength ?? 0.75,
    });
    navigate('create');
    focusPrompt();
  }
  async function removeAsset(asset: Asset) {
    try {
      await api.removeAsset(asset.id);
      await refresh();
      setNotice({ kind: 'success', message: `已删除 ${asset.name}。` });
    } catch (error) {
      fail(error);
    }
  }
  async function cancel(job: Job) {
    try {
      await api.cancel(job.id);
      await refresh();
      setNotice({ kind: 'success', message: '任务已取消。' });
    } catch (error) {
      fail(error);
    }
  }
  async function saveProject(name: string, description: string) {
    if (projectDialog && projectDialog !== 'new')
      await api.updateProject(projectDialog.id, name, description);
    else await api.createProject(name, description);
    await refresh();
  }
  async function archiveProject(project: Project) {
    try {
      await api.archiveProject(project.id, !project.archived);
      if (project.id === projectId && !project.archived) enterWorkspace('');
      await refresh();
    } catch (e) {
      fail(e);
    }
  }
  async function removeProject(project: Project) {
    if (
      !window.confirm(`从侧栏移除「${project.name}」？素材、生成记录和视频文件会保留，不会被删除。`)
    )
      return;
    try {
      await api.removeProject(project.id);
      if (project.id === projectId) enterWorkspace('');
      await refresh();
    } catch (e) {
      fail(e);
    }
  }
  async function archiveJob(job: Job) {
    try {
      await (job.archived ? api.restore(job.id) : api.archive(job.id));
      await refresh();
    } catch (e) {
      fail(e);
    }
  }
  async function deleteJob(job: Job) {
    if (!window.confirm('删除这条生成记录？')) return;
    try {
      await api.deleteJob(job.id);
      await refresh();
    } catch (e) {
      fail(e);
    }
  }
  async function moveJob(job: Job, projectId: string) {
    try {
      await api.moveJob(job.id, projectId);
      await refresh();
    } catch (e) {
      fail(e);
    }
  }
  async function assignProject(asset: Asset, projectId: string) {
    try {
      await api.projectAssets(projectId, asset.id);
      await refresh();
    } catch (e) {
      fail(e);
    }
  }
  const jobProps = {
    onContinue: async (id: string) => {
      try {
        const plan = await api.longVideo(id);
        if (plan.project_id && !projects.some((p) => p.id === plan.project_id)) {
          fail(new Error('该视频所属项目已移除，不能在其他项目中继续编辑。'));
          return;
        }
        enterWorkspace(plan.project_id || '');
        setResumeID(id);
        setStrategy('long');
        navigate('create');
      } catch (e) {
        fail(e);
      }
    },
    jobs: jobs.filter((job) => !projectId || job.project_id === projectId),
    onRefresh: () => void refresh(),
    onReuse: reuse,
    onCancel: cancel,
    onArchive: archiveJob,
    onDelete: deleteJob,
    projects: projects.filter((p) => !p.archived),
    onMove: moveJob,
    locked: busy || agentBusy,
  };
  const isLong =
    strategy === 'long' ||
    (strategy === 'auto' &&
      !!longSettings &&
      (options.duration >= longSettings.threshold ||
        (options.duration >= longSettings.reference_threshold &&
          referencedAssets(prompt, scopedAssets).length >= longSettings.reference_count)));
  return (
    <div className="app-shell">
      <a className="skip-link" href="#main-content">
        跳转到主要内容
      </a>
      <Sidebar
        view={view}
        onNavigate={(next) => enterWorkspace('', next)}
        projects={projects}
        projectId={projectId}
        locked={busy || agentBusy || uploading}
        onOpenProject={enterWorkspace}
        onCreateProject={() => setProjectDialog('new')}
        onEditProject={setProjectDialog}
        onArchiveProject={(project) => void archiveProject(project)}
        onRemoveProject={(project) => void removeProject(project)}
        onSettings={() => setSettings(true)}
        activeCount={jobs.filter((job) => isActiveJob(job.status)).length}
      />
      <main id="main-content" className="workspace">
        <header className="topbar">
          <div className="breadcrumb">
            <span className="workspace-name" title={activeProject?.name}>
              {activeProject?.name || '我的工作室'}
            </span>
            <span>/</span>
            <strong>
              {view === 'create' ? '创作空间' : view === 'assets' ? '素材库' : '生成记录'}
            </strong>
          </div>
          <div
            className={`connection ${connection}`}
            title="此状态仅表示 Local Dream 服务连接，不代表 ComfyUI 可生成"
          >
            <span />
            {connection === 'online'
              ? '本地服务已连接'
              : connection === 'offline'
                ? '本地服务未连接'
                : '正在连接服务'}
          </div>
        </header>
        <div className="content">
          {loadError && (
            <div className="connection-alert" role="status">
              <CircleAlert size={17} />
              <span>{loadError} 已保留当前内容，将自动重连。</span>
              <button className="text-button" onClick={() => void refresh()}>
                重试
              </button>
            </div>
          )}
          {notice && (
            <div
              className={`notice ${notice.kind}`}
              role={notice.kind === 'error' ? 'alert' : 'status'}
            >
              {notice.kind === 'error' ? <CircleAlert size={18} /> : <CheckCircle2 size={18} />}
              <span>{notice.message}</span>
              <button className="icon-button" aria-label="关闭提示" onClick={() => setNotice(null)}>
                <X size={16} />
              </button>
            </div>
          )}
          {view === 'create' ? (
            <>
              {!isLong && (
                <section className="hero">
                  <div className="hero-kicker">
                    <Sparkles size={13} /> YOUR IMAGINATION, IN MOTION
                  </div>
                  <h1>
                    今天，想让什么<span>灵感成真</span>？
                  </h1>
                  <p>一个念头，一帧画面。从这里，开始你的下一段故事。</p>
                </section>
              )}
              <CreationModes
                mode={options.mode}
                long={isLong}
                disabled={busy || agentBusy || uploading}
                onSelect={(mode) => {
                  if (mode === 'long') setStrategy('long');
                  else {
                    setOptions(changeMode(options, mode));
                    setStrategy('normal');
                  }
                }}
              />
              {settingsError && (
                <p className="form-error">
                  无法读取长视频设置：{settingsError}。请确认后端已更新。
                </p>
              )}
              {isLong && longSettings ? (
                <LongVideoEditor
                  key={projectId || 'personal'}
                  projectId={projectId}
                  resumeID={resumeID}
                  settings={longSettings}
                  onSettingsRefresh={setLongSettings}
                  onNewVideo={() => setResumeID('')}
                  assets={assets.filter((a) => !projectId || a.project_id === projectId)}
                  jobs={jobs}
                  initialPrompt={prompt}
                  initialOptions={options}
                  onRefresh={refresh}
                  onUpload={() => fileInput.current?.click()}
                  uploading={uploading}
                />
              ) : (
                <Composer
                  hideModes
                  prompt={prompt}
                  onPrompt={setPrompt}
                  options={options}
                  onOptions={setOptions}
                  assets={assets.filter((a) => !projectId || a.project_id === projectId)}
                  busy={busy}
                  agentBusy={agentBusy}
                  uploading={uploading}
                  onUpload={() => fileInput.current?.click()}
                  onGenerate={() => void generate()}
                  onInterpret={() => void interpret()}
                />
              )}
              {!isLong && (
                <div className="quick-ideas">
                  <span>试着描述</span>
                  {['镜头缓慢推进', '自然光与胶片质感', '一段电影感的长镜头'].map((text) => (
                    <button
                      key={text}
                      disabled={busy || agentBusy}
                      onClick={() => {
                        setPrompt((value) => `${value}${value ? '，' : ''}${text}`);
                        focusPrompt();
                      }}
                    >
                      {text}
                      <ArrowUpRight size={12} />
                    </button>
                  ))}
                </div>
              )}
              {!isLong && <Inspiration onChoose={chooseIdea} disabled={busy || agentBusy} />}
              <Jobs {...jobProps} compact onViewAll={() => navigate('history')} />
            </>
          ) : view === 'assets' ? (
            <AssetLibrary
              assets={scopedAssets}
              onUpload={() => fileInput.current?.click()}
              uploading={uploading}
              onReference={reference}
              onDelete={removeAsset}
              projects={projects.filter((p) => !p.archived)}
              onAssignProject={assignProject}
              locked={busy || agentBusy}
            />
          ) : (
            <Jobs {...jobProps} />
          )}
          <footer className="workspace-footer">
            <span className="footer-brand">
              <Sparkles size={12} /> LOCAL DREAM
            </span>
            <span>让每一个想法，都有自己的画面。</span>
          </footer>
        </div>
      </main>
      <input
        ref={fileInput}
        type="file"
        accept="image/*,video/*,audio/*"
        multiple
        hidden
        aria-label="上传素材文件"
        onChange={(event) => void upload(event.target.files)}
      />
      {settings && (
        <SettingsDialog
          onClose={() => setSettings(false)}
          onLongVideoSettingsSaved={setLongSettings}
        />
      )}
      {projectDialog && (
        <ProjectDialog
          project={projectDialog === 'new' ? undefined : projectDialog}
          onClose={() => setProjectDialog(null)}
          onSave={saveProject}
        />
      )}
    </div>
  );
}
