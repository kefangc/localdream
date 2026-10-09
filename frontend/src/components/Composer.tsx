import { useEffect, useRef, useState } from 'react';
import { ArrowUp, AudioLines, Bot, ImagePlus, Paperclip, Sparkles, Video, X } from 'lucide-react';
import { changeMode, modes, referencedAssets } from '../lib/generation';
import type { Asset, GenerationOptions } from '../types';
import { GenerationControls } from './GenerationControls';
import { FloatingPanel } from './FloatingPanel';
import { caretRect } from '../lib/caret';

type Props = {
  longVideo?: boolean;
  readOnly?: boolean;
  formatLocked?: boolean;
  generateLabel?: string;
  prompt: string;
  onPrompt: (prompt: string) => void;
  options: GenerationOptions;
  onOptions: (options: GenerationOptions) => void;
  assets: Asset[];
  projectId?: string;
  hideModes?: boolean;
  busy: boolean;
  agentBusy: boolean;
  uploading: boolean;
  onUpload: () => void;
  onGenerate: () => void;
  onInterpret: () => void;
};
export function Composer(props: Props) {
  const {
    prompt,
    onPrompt,
    options,
    onOptions,
    assets,
    busy,
    agentBusy,
    uploading,
    onUpload,
    onGenerate,
    onInterpret,
  } = props;
  const [showMentions, setShowMentions] = useState(false);
  const [query, setQuery] = useState('');
  const [active, setActive] = useState(0);
  const [cursor, setCursor] = useState(0);
  const textarea = useRef<HTMLTextAreaElement>(null);
  const selection = useRef({ start: 0, end: 0 });
  const locked = busy || agentBusy || !!props.readOnly;
  const references = referencedAssets(prompt, assets);
  useEffect(() => {
    if (!textarea.current) return;
    textarea.current.style.height = 'auto';
    textarea.current.style.height = `${Math.min(360, Math.max(126, textarea.current.scrollHeight))}px`;
  }, [prompt]);
  const candidates = assets.filter((asset) =>
    asset.name.toLowerCase().includes(query.toLowerCase()),
  );
  useEffect(() => {
    if (showMentions && candidates[active])
      document
        .getElementById(`candidate-${candidates[active].id}`)
        ?.scrollIntoView({ block: 'nearest' });
  }, [active, query, showMentions]);
  function insertMention(asset: Asset) {
    const { start, end } = selection.current;
    const prefix = prompt.slice(0, start);
    const token = `@${query}`;
    const before = prefix.endsWith(token) ? prefix.slice(0, -token.length) : prefix;
    const next = `${before}@${asset.name} `;
    onPrompt(next + prompt.slice(end));
    setShowMentions(false);
    requestAnimationFrame(() => {
      textarea.current?.focus();
      textarea.current?.setSelectionRange(next.length, next.length);
    });
  }
  return (
    <section className="composer-section" aria-label="视频创作编辑器">
      {!props.longVideo && !props.hideModes && (
        <div className="mode-tabs" role="group" aria-label="创作模式">
          {modes.map((mode) => (
            <button
              key={mode.id}
              disabled={locked}
              aria-pressed={options.mode === mode.id}
              className={options.mode === mode.id ? 'selected' : ''}
              onClick={() => onOptions(changeMode(options, mode.id))}
            >
              {mode.id === 'text_to_video' ? (
                <Sparkles size={16} />
              ) : mode.id === 'first_last_frame' ? (
                <ImagePlus size={16} />
              ) : (
                <Video size={16} />
              )}
              {mode.label}
            </button>
          ))}
        </div>
      )}
      <div className="composer">
        <div className="prompt-area">
          <button
            className="reference-add"
            onClick={onUpload}
            disabled={uploading || locked}
            aria-label="上传参考素材"
          >
            <ImagePlus size={25} />
            <span>{uploading ? '上传中' : '添加参考'}</span>
          </button>
          <textarea
            id="creative-prompt"
            ref={textarea}
            aria-label="画面描述"
            aria-autocomplete="list"
            aria-controls={showMentions ? 'asset-candidates' : undefined}
            aria-activedescendant={
              showMentions && candidates[active] ? `candidate-${candidates[active].id}` : undefined
            }
            disabled={locked}
            value={prompt}
            placeholder={
              '把脑海中的画面，变成眼前的故事。\n描述你的想法，或使用 @ 引用图片、视频和音频…'
            }
            onSelect={(event) => {
              selection.current = {
                start: event.currentTarget.selectionStart,
                end: event.currentTarget.selectionEnd,
              };
              setCursor(event.currentTarget.selectionEnd);
            }}
            onChange={(event) => {
              onPrompt(event.target.value);
              selection.current = {
                start: event.target.selectionStart,
                end: event.target.selectionEnd,
              };
              const match = event.target.value
                .slice(0, event.target.selectionStart)
                .match(/@([^@\n]*)$/);
              const completed =
                match && assets.some((asset) => match[1].startsWith(`${asset.name} `));
              setShowMentions(!!match && !completed);
              setQuery(match?.[1] ?? '');
              setActive(0);
              setCursor(event.target.selectionEnd);
            }}
            onKeyDown={(event) => {
              if (event.nativeEvent.isComposing) return;
              if (
                showMentions &&
                ['ArrowDown', 'ArrowUp', 'Enter'].includes(event.key) &&
                !event.ctrlKey &&
                !event.metaKey
              ) {
                event.preventDefault();
                if (event.key === 'Enter' && candidates[active]) insertMention(candidates[active]);
                else
                  setActive(
                    (current) =>
                      (current +
                        (event.key === 'ArrowUp' ? -1 : 1) +
                        Math.max(1, candidates.length)) %
                      Math.max(1, candidates.length),
                  );
                return;
              }
              if (event.key === 'Escape') setShowMentions(false);
              if (
                (event.ctrlKey || event.metaKey) &&
                event.key === 'Enter' &&
                !event.nativeEvent.isComposing &&
                prompt.trim() &&
                !locked &&
                !uploading
              ) {
                event.preventDefault();
                onGenerate();
              }
            }}
          />
        </div>
        {references.length > 0 && (
          <div className="reference-chips" aria-label="已引用素材">
            {references.map((asset, index) => (
              <span key={asset.id}>
                {index + 1}
                <Paperclip size={12} />
                {asset.name}
              </span>
            ))}
          </div>
        )}
        <div className="composer-toolbar">
          <div className="composer-tools">
            <span className="model-label">
              <span className="mint-dot" />
              MiniMax H3
            </span>
            {!props.longVideo && (
              <button
                className={`tool-button ${options.use_agent ? 'accent-text' : ''}`}
                role="switch"
                aria-checked={options.use_agent}
                disabled={locked}
                onClick={() => onOptions({ ...options, use_agent: !options.use_agent })}
              >
                <Bot size={16} />
                Agent <span className={`mini-switch ${options.use_agent ? 'on' : ''}`} />
              </button>
            )}
            <button
              className="tool-button"
              disabled={locked}
              aria-label="引用素材"
              data-floating-trigger
              aria-expanded={showMentions}
              onClick={() => {
                setQuery('');
                setActive(0);
                setShowMentions(!showMentions);
              }}
            >
              {' '}
              @{' '}
            </button>
            <GenerationControls
              value={options}
              onChange={onOptions}
              disabled={locked}
              longVideo={props.longVideo}
              formatLocked={props.formatLocked}
            />
          </div>
          <div className="submit-tools">
            {options.use_agent && (
              <button
                className="tool-button accent-text"
                disabled={locked || !prompt.trim()}
                onClick={onInterpret}
              >
                <Sparkles size={15} />
                {agentBusy ? '完善中…' : '完善描述'}
              </button>
            )}
            <button
              className="generate-button"
              disabled={locked || uploading || !prompt.trim()}
              onClick={onGenerate}
            >
              {busy ? '提交中…' : props.generateLabel || '生成视频'}
              <ArrowUp size={18} />
            </button>
          </div>
        </div>
        {showMentions && !locked && (
          <FloatingPanel
            label="素材候选"
            className="caret-mentions"
            anchor={() => caretRect(textarea.current!, cursor)}
            onClose={() => setShowMentions(false)}
          >
            <div className="popover-heading">
              <strong>可能 @ 的内容</strong>
              <button
                className="icon-button"
                aria-label="关闭素材引用"
                onClick={() => setShowMentions(false)}
              >
                <X size={15} />
              </button>
            </div>
            <div id="asset-candidates" role="listbox" aria-label="参考素材">
              {candidates.length ? (
                candidates.map((asset, index) => (
                  <button
                    id={`candidate-${asset.id}`}
                    role="option"
                    aria-selected={active === index}
                    key={asset.id}
                    disabled={locked}
                    onMouseDown={(event) => event.preventDefault()}
                    onClick={() => insertMention(asset)}
                  >
                    {asset.type === 'audio' ? (
                      <AudioLines size={16} />
                    ) : asset.type === 'video' ? (
                      <Video size={16} />
                    ) : (
                      <img className="mention-thumbnail" src={asset.url} alt="" />
                    )}
                    <span>{asset.name}</span>
                  </button>
                ))
              ) : (
                <p>
                  {assets.length
                    ? '没有匹配素材，试试其他名称。'
                    : '还没有素材。先点击「添加参考」上传。'}
                </p>
              )}
            </div>
          </FloatingPanel>
        )}
      </div>
      <div className="composer-caption">
        <span>
          {props.longVideo
            ? '引用素材可选 · 时长为裁帧后的有效时长'
            : modes.find((mode) => mode.id === options.mode)?.hint}
        </span>
        <span>
          {options.duration}s <i>·</i> {options.aspect_ratio} <i>·</i> {options.resolution}
          <kbd>Ctrl ↵</kbd>
        </span>
      </div>
      {options.use_agent && (
        <p className="privacy-note">
          Agent 会将描述与素材信息发送至你配置的模型服务；视频生成由本地 ComfyUI 执行。
        </p>
      )}
    </section>
  );
}
