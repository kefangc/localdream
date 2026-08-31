import React, { useEffect, useRef, useState } from 'react';
import { createRoot } from 'react-dom/client';
import {
  Bot,
  ChevronDown,
  Clock3,
  Film,
  ImagePlus,
  LayoutGrid,
  MoreHorizontal,
  Play,
  Plus,
  Send,
  Settings2,
  Sparkles,
  Trash2,
  Upload,
  Video,
  WandSparkles,
} from 'lucide-react';
import './styles.css';
import './overrides.css';

type Asset = { id: string; name: string; type: string; url: string; size: number };
type Job = {
  id: string;
  mode: string;
  status: string;
  prompt: string;
  duration: number;
  aspect_ratio: string;
  resolution: string;
  steps: number;
  lora?: string;
  lora_strength?: number;
  progress: number;
  error?: string;
  output_url?: string;
  asset_ids: string[];
};
const modes = [
  ['text_to_video', '文生视频'],
  ['image_to_video', '图生视频'],
  ['first_last_frame', '首尾帧'],
  ['multi_reference_video', '多参考视频'],
];
const ratios = ['16:9', '9:16', '1:1', '4:3'];
const resolutions = ['480p', '720p'];
const accelerationByMode: Record<string, { value: string; label: string; steps?: number }[]> = {
  text_to_video: [
    { value: '', label: '标准（自定义步数）' },
    { value: 'fl2v_4', label: 'FL2V 4 步 LoRA', steps: 4 },
    { value: 'fl2v_8', label: 'FL2V 8 步 LoRA', steps: 8 },
  ],
  image_to_video: [
    { value: '', label: '标准（自定义步数）' },
    { value: 'ref2v_4', label: 'Ref2V 4 步 LoRA', steps: 4 },
  ],
  first_last_frame: [
    { value: '', label: '标准（自定义步数）' },
    { value: 'fl2v_4', label: 'FL2V 4 步 LoRA', steps: 4 },
    { value: 'fl2v_8', label: 'FL2V 8 步 LoRA', steps: 8 },
  ],
  multi_reference_video: [
    { value: '', label: '标准（自定义步数）' },
    { value: 'ref2v_4', label: 'Ref2V 4 步 LoRA', steps: 4 },
  ],
};
function App() {
  const [assets, setAssets] = useState<Asset[]>([]),
    [jobs, setJobs] = useState<Job[]>([]),
    [mode, setMode] = useState('image_to_video'),
    [duration, setDuration] = useState(5),
    [ratio, setRatio] = useState('16:9'),
    [resolution, setResolution] = useState('720p'),
    [steps, setSteps] = useState(4),
    [lora, setLora] = useState(''),
    [loraStrength, setLoraStrength] = useState(0.75),
    [prompt, setPrompt] = useState(''),
    [agent, setAgent] = useState(true),
    [mention, setMention] = useState(false),
    [busy, setBusy] = useState(false),
    [agentBusy, setAgentBusy] = useState(false),
    [settings, setSettings] = useState(false),
    [baseUrl, setBaseUrl] = useState(''),
    [apiKey, setApiKey] = useState(''),
    [model, setModel] = useState(''),
    [models, setModels] = useState<string[]>([]);
  const fileRef = useRef<HTMLInputElement>(null);
  const promptRef = useRef<HTMLTextAreaElement>(null);
  useEffect(() => {
    fetch('/api/assets')
      .then((r) => r.json())
      .then(setAssets)
      .catch(() => {});
    fetch('/api/generations')
      .then((r) => r.json())
      .then(setJobs)
      .catch(() => {});
  }, []);
  useEffect(() => {
    const t = setInterval(
      () =>
        fetch('/api/generations')
          .then((r) => r.json())
          .then(setJobs)
          .catch(() => {}),
      1500,
    );
    return () => clearInterval(t);
  }, []);
  async function upload(files: FileList | null) {
    if (!files) return;
    for (const file of Array.from(files)) {
      const fd = new FormData();
      fd.append('file', file);
      const res = await fetch('/api/assets', { method: 'POST', body: fd });
      if (res.ok) {
        const a = await res.json();
        setAssets((v) => [a, ...v]);
      }
    }
  }
  function onPrompt(v: string) {
    setPrompt(v);
    setMention(v.slice(-1) === '@');
  }
  function chooseMode(next: string) {
    setMode(next);
    setLora('');
  }
  function chooseAcceleration(next: string) {
    setLora(next);
    const selected = (accelerationByMode[mode] || []).find((item) => item.value === next);
    if (selected?.steps) setSteps(selected.steps);
  }
  function addMention(a: Asset) {
    const base = prompt.endsWith('@') ? prompt.slice(0, -1) : prompt;
    setPrompt(`${base}@${a.name} `);
    setMention(false);
    requestAnimationFrame(() => {
      promptRef.current?.focus();
      const end = promptRef.current?.value.length ?? 0;
      promptRef.current?.setSelectionRange(end, end);
    });
  }
  async function generate() {
    setBusy(true);
    const assetIds = assets
      .filter((a) => prompt.includes(a.name))
      .sort((a, b) => prompt.indexOf(a.name) - prompt.indexOf(b.name))
      .map((a) => a.id);
    const res = await fetch('/api/generations', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        mode,
        prompt,
        duration,
        aspect_ratio: ratio,
        resolution,
        steps,
        lora,
        lora_strength: lora ? loraStrength : 0,
        use_agent: agent,
        asset_ids: assetIds,
      }),
    });
    if (res.ok) {
      const j = await res.json();
      setJobs((v) => [j, ...v]);
      setPrompt('');
    } else {
      const e = await res.json();
      alert(e.error);
    }
    setBusy(false);
  }
  async function getModels() {
    const r = await fetch('/api/providers/models', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ base_url: baseUrl, api_key: apiKey }),
    });
    const d = await r.json();
    if (r.ok && Array.isArray(d.models) && d.models.length) {
      setModels(d.models);
      if (!d.models.includes(model)) setModel('');
    } else alert(d.error || '获取模型失败');
  }
  async function saveProvider() {
    const r = await fetch('/api/config/provider', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ base_url: baseUrl, api_key: apiKey, model }),
    });
    if (!r.ok) {
      const d = await r.json();
      alert(d.error || '保存失败');
    } else {
      setApiKey('');
      alert('配置已保存到本机');
    }
  }
  async function interpret() {
    if (!prompt.trim()) {
      alert('先输入你的想法');
      return;
    }
    setAgentBusy(true);
    const r = await fetch('/api/agent/interpret', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ prompt, assets, base_url: baseUrl, api_key: apiKey, model }),
    });
    const d = await r.json();
    if (r.ok) {
      if (d.mode) setMode(d.mode);
      if (d.duration) setDuration(Number(d.duration));
      if (d.aspect_ratio) setRatio(d.aspect_ratio);
      if (d.resolution) setResolution(d.resolution);
      if (d.prompt) {
        const p = d.prompt;
        setPrompt(
          [p.subject, p.motion, p.camera, p.environment, p.constraints].filter(Boolean).join('；'),
        );
      }
    } else alert(d.error || 'Agent 解析失败');
    setAgentBusy(false);
  }
  async function removeAsset(id: string) {
    const res = await fetch(`/api/assets/${encodeURIComponent(id)}`, { method: 'DELETE' });
    if (res.ok || res.status === 204) setAssets((v) => v.filter((a) => a.id !== id));
    else alert('删除素材失败');
  }
  return (
    <div className="app-shell">
      <aside className="rail">
        <div className="brand-mark">LD</div>
        <button className="rail-item active">
          <WandSparkles size={18} />
          <span>创作</span>
        </button>
        <button className="rail-item">
          <LayoutGrid size={18} />
          <span>资产</span>
        </button>
        <button className="rail-item">
          <Clock3 size={18} />
          <span>历史</span>
        </button>
        <div className="rail-spacer" />
        <button className="rail-item" onClick={() => setSettings((v) => !v)}>
          <Settings2 size={18} />
          <span>设置</span>
        </button>
      </aside>
      <main className="workspace">
        <header className="topbar">
          <div>
            <div className="eyebrow">LOCAL DREAM STUDIO</div>
            <h1>视频创作</h1>
          </div>
          <div className="top-actions">
            <span className="status-dot" />
            本地 ComfyUI{' '}
            <button className="icon-btn" title="更多">
              <MoreHorizontal size={18} />
            </button>
          </div>
        </header>
        <section className="creator">
          <div className="section-head">
            <div>
              <p className="kicker">把想法变成镜头</p>
              <h2>选择你的创作方式</h2>
            </div>
            <div className="agent-toggle">
              <Bot size={16} />
              <span>Agent 模式</span>
              <button
                className={agent ? 'switch on' : 'switch'}
                onClick={() => setAgent((v) => !v)}
              >
                <span />
              </button>
            </div>
          </div>
          <div className="mode-row">
            {modes.map(([id, label]) => (
              <button
                key={id}
                className={mode === id ? 'mode-btn selected' : 'mode-btn'}
                onClick={() => chooseMode(id)}
              >
                {id === 'text_to_video' ? (
                  <Sparkles size={16} />
                ) : id === 'first_last_frame' ? (
                  <Film size={16} />
                ) : (
                  <Video size={16} />
                )}
                <span>{label}</span>
              </button>
            ))}
          </div>
          <div className="control-grid">
            <label>
              模型
              <select>
                <option>MiniMax H3 · 本地</option>
              </select>
            </label>
            <label>
              时长
              <div className="segmented">
                {[5, 10, 15].map((v) => (
                  <button
                    className={duration === v ? 'chosen' : ''}
                    key={v}
                    onClick={() => setDuration(v)}
                  >
                    {v}s
                  </button>
                ))}
              </div>
            </label>
            <label>
              比例
              <div className="segmented">
                {ratios.map((v) => (
                  <button
                    className={ratio === v ? 'chosen' : ''}
                    key={v}
                    onClick={() => setRatio(v)}
                  >
                    {v}
                  </button>
                ))}
              </div>
            </label>
            <label>
              分辨率
              <div className="segmented">
                {resolutions.map((v) => (
                  <button
                    className={resolution === v ? 'chosen' : ''}
                    key={v}
                    onClick={() => setResolution(v)}
                  >
                    {v}
                  </button>
                ))}
              </div>
            </label>
            <label>
              采样步数
              <input
                type="number"
                min="1"
                max="100"
                value={steps}
                disabled={!!lora}
                onChange={(e) => setSteps(Math.max(1, Math.min(100, Number(e.target.value) || 1)))}
              />
            </label>
            <label>
              LoRA 加速
              <select value={lora} onChange={(e) => chooseAcceleration(e.target.value)}>
                {(accelerationByMode[mode] || []).map((item) => (
                  <option key={item.value} value={item.value}>
                    {item.label}
                  </option>
                ))}
              </select>
            </label>
            <label>
              LoRA 强度
              <input
                type="number"
                min="0"
                max="1"
                step="0.05"
                value={loraStrength}
                disabled={!lora}
                onChange={(e) =>
                  setLoraStrength(Math.max(0, Math.min(1, Number(e.target.value) || 0)))
                }
              />
            </label>
          </div>
          <div className="prompt-wrap">
            <textarea
              ref={promptRef}
              value={prompt}
              onChange={(e) => onPrompt(e.target.value)}
              placeholder="描述你想看到的画面，使用 @ 引用素材，例如：@图片1 中的人物从画面左侧走向镜头……"
            />
            <div className="prompt-tools">
              <button className="tool-btn" onClick={() => fileRef.current?.click()}>
                <Upload size={16} />
                添加素材
              </button>
              {agent && (
                <button className="tool-btn agent-action" disabled={agentBusy} onClick={interpret}>
                  <Sparkles size={15} />
                  {agentBusy ? '理解中' : 'Agent 完善'}
                </button>
              )}
              <span>{agent ? 'Agent 将补全镜头、动作与环境细节' : '手动提示词'}</span>
              <button className="send-btn" disabled={busy} onClick={generate}>
                {busy ? '提交中' : '生成视频'}
                <Send size={16} />
              </button>
            </div>
            {mention && (
              <div className="mention-menu">
                {assets.length ? (
                  assets.map((a) => (
                    <button
                      key={a.id}
                      onMouseDown={(e) => e.preventDefault()}
                      onClick={() => addMention(a)}
                    >
                      {a.type === 'video' ? <Video size={15} /> : <ImagePlus size={15} />}
                      <span>{a.name}</span>
                    </button>
                  ))
                ) : (
                  <div className="empty-mention">先添加图片或视频素材</div>
                )}
              </div>
            )}
          </div>
          <input
            ref={fileRef}
            type="file"
            accept="image/*,video/*"
            multiple
            hidden
            onChange={(e) => upload(e.target.files)}
          />
          <div className="asset-strip">
            <div className="asset-label">
              参考素材 <span>{assets.length}/12</span>
            </div>
            <div className="asset-list">
              <button className="upload-tile" onClick={() => fileRef.current?.click()}>
                <Plus size={20} />
                <span>上传</span>
              </button>
              {assets.map((a) => (
                <div className="asset-thumb" key={a.id}>
                  <div className="thumb-art" style={{ backgroundImage: `url(${a.url})` }}>
                    {a.type === 'video' && <Video size={15} />}
                  </div>
                  <span title={a.name}>{a.name}</span>
                  <button onClick={() => removeAsset(a.id)}>
                    <Trash2 size={12} />
                  </button>
                </div>
              ))}
            </div>
          </div>
        </section>
        {settings && (
          <section className="settings-panel">
            <div className="settings-title">
              <div>
                <p className="kicker">Agent Provider</p>
                <h3>OpenAI 兼容接口</h3>
              </div>
              <button className="icon-btn" onClick={() => setSettings(false)}>
                ×
              </button>
            </div>
            <div className="settings-fields">
              <label>
                Base URL
                <input
                  value={baseUrl}
                  onChange={(e) => setBaseUrl(e.target.value)}
                  placeholder="https://api.example.com/v1"
                />
              </label>
              <label>
                API Key
                <input
                  type="password"
                  value={apiKey}
                  onChange={(e) => setApiKey(e.target.value)}
                  placeholder="只保存在本机配置"
                />
              </label>
              <label>
                选择模型
                <select
                  value={models.includes(model) ? model : ''}
                  onChange={(e) => setModel(e.target.value)}
                  disabled={!models.length}
                >
                  <option value="">{models.length ? '请选择模型' : '先获取模型列表'}</option>
                  {models.map((item) => (
                    <option key={item} value={item}>
                      {item}
                    </option>
                  ))}
                </select>
              </label>
              <label>
                手动模型名称
                <input
                  value={model}
                  onChange={(e) => setModel(e.target.value)}
                  placeholder="也可以手动输入"
                />
              </label>
              <button className="outline-btn" onClick={getModels}>
                从 /models 获取
              </button>
            </div>
          </section>
        )}
      </main>
      <aside className="jobs-panel">
        <div className="jobs-head">
          <div>
            <p className="kicker">OUTPUT QUEUE</p>
            <h2>任务与作品</h2>
          </div>
          <button
            className="icon-btn"
            title="刷新"
            onClick={() =>
              fetch('/api/generations')
                .then((r) => r.json())
                .then(setJobs)
            }
          >
            <MoreHorizontal size={18} />
          </button>
        </div>
        <div className="jobs-list">
          {jobs.length === 0 ? (
            <div className="empty-jobs">
              <div className="empty-icon">
                <Play size={20} />
              </div>
              <h3>还没有生成任务</h3>
              <p>提交第一个视频后，进度和预览会出现在这里。</p>
            </div>
          ) : (
            jobs.map((j) => <JobCard key={j.id} job={j} />)
          )}
        </div>
      </aside>
    </div>
  );
}
function JobCard({ job }: { job: Job }) {
  return (
    <article className="job-card">
      <div className="job-preview">
        {job.output_url ? (
          <video src={job.output_url} controls />
        ) : (
          <div className="preview-placeholder">
            <Video size={22} />
            {job.status === 'failed'
              ? '生成失败'
              : job.status === 'completed'
                ? '已完成'
                : '处理中'}
          </div>
        )}
        <span className={`job-status ${job.status}`}>
          {job.status === 'queued'
            ? '排队中'
            : job.status === 'running'
              ? '生成中'
              : job.status === 'completed'
                ? '完成'
                : '失败'}
        </span>
      </div>
      <div className="job-meta">
        <div>
          <strong>
            {job.mode === 'first_last_frame'
              ? '首尾帧'
              : job.mode === 'multi_reference_video'
                ? '多参考视频'
                : job.mode === 'text_to_video'
                  ? '文生视频'
                  : '图生视频'}
          </strong>
          <span>
            {job.duration}s · {job.resolution} · {job.aspect_ratio} · {job.steps} 步
            {job.lora ? ' · LoRA' : ''}
          </span>
        </div>
        <span className="job-id">{job.id}</span>
      </div>
      {job.status === 'running' && (
        <div className="progress-row">
          <div className="progress">
            <span style={{ width: `${job.progress}%` }} />
          </div>
          <span>{job.progress}%</span>
        </div>
      )}
      {job.error && <p className="job-error">{job.error}</p>}
    </article>
  );
}
createRoot(document.getElementById('root')!).render(<App />);
