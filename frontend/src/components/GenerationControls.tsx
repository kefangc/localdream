import { useRef, useState } from 'react';
import { Clock3, SlidersHorizontal } from 'lucide-react';
import { ratios, resolutions } from '../lib/generation';
import type { GenerationOptions } from '../types';
import { FloatingPanel } from './FloatingPanel';
import { Parameters } from './Parameters';

export function GenerationControls({
  value,
  onChange,
  disabled,
  longVideo = false,
  formatLocked = false,
}: {
  value: GenerationOptions;
  onChange: (value: GenerationOptions) => void;
  disabled: boolean;
  longVideo?: boolean;
  formatLocked?: boolean;
}) {
  const [open, setOpen] = useState<'format' | 'duration' | 'advanced' | null>(null);
  const [durationDraft, setDurationDraft] = useState<string | null>(null);
  const [resolutionDraft, setResolutionDraft] = useState<string | null>(null);
  const megapixels = value.resolution.endsWith('MP')
    ? Number.parseFloat(value.resolution)
    : value.resolution === '480p'
      ? 0.4
      : 0.9;
  const setMegapixels = (next: number) =>
    onChange({ ...value, resolution: `${Math.max(0.1, Math.min(1.9, next)).toFixed(1)}MP` });
  const format = useRef<HTMLButtonElement>(null);
  const duration = useRef<HTMLButtonElement>(null);
  const advanced = useRef<HTMLButtonElement>(null);
  const refs = { format, duration, advanced };
  const close = () => setOpen(null);
  const update = (patch: Partial<GenerationOptions>) => onChange({ ...value, ...patch });
  const toggle = (next: typeof open) => setOpen(open === next ? null : next);
  const shape = (ratio: string) => {
    const [width, height] = ratio.split(':').map(Number);
    return {
      width: (22 * width) / Math.max(width, height),
      height: (22 * height) / Math.max(width, height),
    };
  };
  return (
    <div className="generation-controls">
      <button
        ref={format}
        data-floating-trigger
        className={`tool-button parameter-trigger ${open === 'format' ? 'active' : ''}`}
        disabled={disabled || formatLocked}
        title={formatLocked ? '当前 latent 续接需保持比例与分辨率一致' : undefined}
        aria-label="比例与分辨率"
        aria-expanded={open === 'format'}
        onClick={() => toggle('format')}
      >
        <span className="ratio-outline" style={shape(value.aspect_ratio)} />
        {value.aspect_ratio}
        <span className="parameter-divider">·</span>
        {value.resolution.toUpperCase()}
      </button>
      <button
        ref={duration}
        data-floating-trigger
        className={`tool-button parameter-trigger ${open === 'duration' ? 'active' : ''}`}
        disabled={disabled}
        aria-label="视频时长"
        aria-expanded={open === 'duration'}
        onClick={() => toggle('duration')}
      >
        <Clock3 size={15} />
        {value.duration}s
      </button>
      <button
        ref={advanced}
        data-floating-trigger
        className="tool-button"
        aria-label="高级参数"
        aria-expanded={open === 'advanced'}
        disabled={disabled}
        onClick={() => toggle('advanced')}
      >
        <SlidersHorizontal size={15} />
      </button>
      {open && !disabled && (
        <FloatingPanel
          key={open}
          placement={open === 'advanced' ? 'bottom' : 'auto'}
          label={open === 'format' ? '画面设置' : open === 'duration' ? '时长设置' : '高级设置'}
          className={`parameter-panel ${open}`}
          anchor={() => refs[open].current!.getBoundingClientRect()}
          onClose={close}
        >
          {open === 'format' ? (
            <>
              <div className="parameter-label">选择比例</div>
              <div className="visual-segments ratio-options" role="group" aria-label="选择比例">
                {ratios.map((ratio) => (
                  <button
                    key={ratio}
                    aria-pressed={value.aspect_ratio === ratio}
                    onClick={() => update({ aspect_ratio: ratio })}
                  >
                    <span className="ratio-icon-box">
                      <span className="ratio-outline" style={shape(ratio)} />
                    </span>
                    {ratio}
                  </button>
                ))}
              </div>
              <div className="parameter-label">选择分辨率</div>
              <div className="visual-segments" role="group" aria-label="选择分辨率">
                {resolutions.map((resolution) => (
                  <button
                    key={resolution}
                    aria-pressed={value.resolution === resolution}
                    onClick={() => {
                      setResolutionDraft(null);
                      update({ resolution });
                    }}
                  >
                    {resolution.toUpperCase()}
                  </button>
                ))}
              </div>
              <div className="parameter-label">自定义分辨率（百万像素）</div>
              <div className="duration-editor">
                <div>
                  <input
                    aria-label="自定义分辨率滑块"
                    type="range"
                    min={0.1}
                    max={1.9}
                    step={0.1}
                    value={megapixels}
                    onChange={(event) => {
                      setResolutionDraft(null);
                      setMegapixels(Number(event.target.value));
                    }}
                  />
                  <div className="duration-ticks">
                    {[0.1, 0.5, 1.0, 1.5, 1.9].map((mp) => (
                      <button
                        key={mp}
                        onClick={() => {
                          setResolutionDraft(null);
                          setMegapixels(mp);
                        }}
                      >
                        {mp.toFixed(1)}
                      </button>
                    ))}
                  </div>
                </div>
                <label className="duration-number">
                  <input
                    aria-label="自定义分辨率百万像素"
                    type="number"
                    min={0.1}
                    max={1.9}
                    step={0.1}
                    value={resolutionDraft ?? megapixels}
                    onChange={(event) => {
                      setResolutionDraft(event.target.value);
                      const next = Number(event.target.value);
                      if (event.target.value && Number.isFinite(next) && next >= 0.1 && next <= 1.9)
                        setMegapixels(next);
                    }}
                    onBlur={() => {
                      if (resolutionDraft !== null && resolutionDraft !== '') {
                        const next = Number(resolutionDraft);
                        if (Number.isFinite(next)) setMegapixels(next);
                      }
                      setResolutionDraft(null);
                    }}
                  />
                  <span>MP</span>
                </label>
              </div>
              <p className="parameter-footnote">0.1–1.9 MP · 步长 0.1，实际宽高随比例调整</p>
            </>
          ) : open === 'duration' ? (
            <>
              <div className="parameter-label">
                {longVideo ? '片段有效时长（自动补偿重叠帧）' : '选择视频生成时长'}
              </div>
              <div className="duration-editor">
                <div>
                  <input
                    aria-label="视频时长滑块"
                    type="range"
                    min={1}
                    max={longVideo ? Math.max(30, value.duration) : 15}
                    step={0.1}
                    value={value.duration}
                    onChange={(event) => {
                      setDurationDraft(null);
                      update({ duration: Number(event.target.value) });
                    }}
                  />
                  <div className="duration-ticks">
                    {[1, 5, 10, 15].map((seconds) => (
                      <button
                        key={seconds}
                        onClick={() => {
                          setDurationDraft(null);
                          update({ duration: seconds });
                        }}
                      >
                        {seconds}
                      </button>
                    ))}
                  </div>
                </div>
                <label className="duration-number">
                  <input
                    aria-label="视频时长秒数"
                    type="number"
                    min={1}
                    max={longVideo ? undefined : 15}
                    step="any"
                    value={durationDraft ?? value.duration}
                    onChange={(event) => {
                      // Preserve intermediate input (empty text or a decimal point) while editing.
                      setDurationDraft(event.target.value);
                      const next = Number(event.target.value);
                      if (
                        event.target.value &&
                        Number.isFinite(next) &&
                        next >= 1 &&
                        (longVideo || next <= 15)
                      )
                        update({ duration: next });
                    }}
                    onBlur={() => {
                      if (durationDraft !== null && durationDraft !== '') {
                        const next = Number(durationDraft);
                        if (Number.isFinite(next))
                          update({ duration: Math.max(1, longVideo ? next : Math.min(15, next)) });
                      }
                      setDurationDraft(null);
                    }}
                  />
                  <span>s</span>
                </label>
              </div>
              <p className="parameter-footnote">
                {longVideo
                  ? '手动分段不受 8 秒建议上限约束；仍受模型最大帧数和显存限制。'
                  : '1–15 秒 · 支持小数输入，滑块步长 0.1 秒'}
              </p>
            </>
          ) : (
            <Parameters value={value} onChange={onChange} disabled={disabled} />
          )}
        </FloatingPanel>
      )}
    </div>
  );
}
