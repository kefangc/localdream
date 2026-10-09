import { accelerationByMode } from '../lib/generation';
import type { GenerationOptions } from '../types';

type Props = {
  value: GenerationOptions;
  onChange: (value: GenerationOptions) => void;
  disabled: boolean;
};
export function Parameters({ value, onChange, disabled }: Props) {
  // Controlled components receive values from their parent; there is no duplicate form state.
  const update = (patch: Partial<GenerationOptions>) => onChange({ ...value, ...patch });
  return (
    <fieldset className="parameters" disabled={disabled}>
      <legend className="sr-only">生成参数</legend>
      <details className="advanced" open>
        <summary>高级参数</summary>
        <div className="advanced-grid">
          {value.mode === 'multi_reference_video' && (
            <label>
              参考视频 FPS
              <input
                type="number"
                min={0}
                max={60}
                step={0.1}
                value={value.reference_video_fps ?? 7}
                onChange={(event) => update({ reference_video_fps: Number(event.target.value) })}
              />
            </label>
          )}
          <label>
            LoRA 加速
            <select
              value={value.lora}
              onChange={(event) => {
                const selected = accelerationByMode[value.mode].find(
                  (item) => item.value === event.target.value,
                );
                update({ lora: event.target.value, steps: selected?.steps ?? value.steps });
              }}
            >
              {accelerationByMode[value.mode].map((item) => (
                <option key={item.value} value={item.value}>
                  {item.label}
                </option>
              ))}
            </select>
          </label>
          <label>
            采样步数
            <input
              type="number"
              min={1}
              max={100}
              value={value.steps}
              disabled={!!value.lora}
              onChange={(event) =>
                update({ steps: Math.max(1, Math.min(100, Number(event.target.value) || 1)) })
              }
            />
          </label>
          <label>
            LoRA 强度
            <input
              type="number"
              min={0}
              max={1}
              step={0.05}
              value={value.lora_strength}
              disabled={!value.lora}
              onChange={(event) =>
                update({ lora_strength: Math.max(0, Math.min(1, Number(event.target.value) || 0)) })
              }
            />
          </label>
        </div>
        <p>加速 LoRA 随创作模式切换；素材数量限制以本地工作流实际能力为准。</p>
        {value.mode === 'multi_reference_video' && (
          <p>
            参考视频默认按 7fps 载入，可设 1–60；0 使用源帧率。成片保持 24fps。 H3
            会截短参考帧序列，短片段低帧率可能丢失动作，请检查结果后调整。
          </p>
        )}
      </details>
    </fieldset>
  );
}
