import { useEffect, useState } from 'react';
import { Save } from 'lucide-react';
import { api } from '../lib/api';
import type { LongVideoSettings } from '../types';

export function LongVideoSettingsPanel({
  onSaved,
}: {
  onSaved?: (settings: LongVideoSettings) => void;
}) {
  const [value, setValue] = useState<LongVideoSettings>();
  const [error, setError] = useState('');
  const [message, setMessage] = useState('');
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    api
      .longVideoSettings()
      .then(setValue)
      .catch((e) => setError(e.message));
  }, []);
  async function save() {
    if (!value) return;
    setBusy(true);
    setError('');
    setMessage('');
    try {
      const saved = await api.saveLongVideoSettings(value);
      setValue(saved);
      onSaved?.(saved);
      setMessage('已保存。仅新视频使用这些设置，已有视频保留创建时的配置。');
    } catch (e) {
      setError(e instanceof Error ? e.message : '保存失败');
    } finally {
      setBusy(false);
    }
  }
  const fields: {
    key: keyof LongVideoSettings;
    label: string;
    step: number;
    min: number;
    max?: number;
  }[] = [
    { key: 'threshold', label: '长视频阈值（秒，含等于）', step: 0.1, min: 0.1 },
    { key: 'reference_threshold', label: '多参考触发时长（秒）', step: 0.1, min: 0.1 },
    { key: 'reference_count', label: '多参考触发素材数量', step: 1, min: 1 },
    { key: 'default_seconds', label: '新片段默认有效时长（秒）', step: 0.1, min: 1 },
    { key: 'default_segments', label: '新视频默认片段数', step: 1, min: 1, max: 100 },
    { key: 'max_input_seconds', label: '建议分段输入时长（秒）', step: 0.1, min: 1 },
    {
      key: 'audio_context_frames',
      label: '音频上下文帧数（0 跟随画面）',
      step: 1,
      min: 0,
      max: 240,
    },
  ];
  return (
    <section className="long-settings">
      <h3>长视频与连续生成</h3>
      <p className="muted">手动决定每段内容。所有时长均以有效输出为目标，后端自动补偿。</p>
      {value && (
        <form
          onSubmit={(e) => {
            e.preventDefault();
            void save();
          }}
        >
          <fieldset className="settings-fields" disabled={busy}>
            <label>
              默认生成策略
              <select
                value={value.strategy}
                onChange={(e) =>
                  setValue({ ...value, strategy: e.target.value as LongVideoSettings['strategy'] })
                }
              >
                <option value="auto">自动判断</option>
                <option value="normal">普通单段</option>
                <option value="long">长视频分段</option>
              </select>
            </label>
            <div className="long-settings-grid">
              {fields.map((field) => (
                <label key={field.key}>
                  {field.label}
                  <input
                    required
                    type="number"
                    min={field.min}
                    max={field.max}
                    step={field.step}
                    value={Number(value[field.key])}
                    onChange={(e) => setValue({ ...value, [field.key]: Number(e.target.value) })}
                  />
                </label>
              ))}
              <label>
                画面续接重叠帧数
                <select
                  value={value.context_frames}
                  onChange={(e) => setValue({ ...value, context_frames: Number(e.target.value) })}
                >
                  {[5, 22, 39, 56].map((n) => (
                    <option key={n} value={n}>
                      {n} 帧 · {(n / 24).toFixed(3)} 秒
                    </option>
                  ))}
                </select>
              </label>
            </div>
            <label className="long-check">
              <input
                type="checkbox"
                checked={value.match_audio_tail}
                onChange={(e) => setValue({ ...value, match_audio_tail: e.target.checked })}
              />{' '}
              对齐音频尾部长度，避免逐段累计偏移
            </label>
            <label className="long-check">
              <input
                type="checkbox"
                checked={value.use_te_speed}
                onChange={(e) => setValue({ ...value, use_te_speed: e.target.checked })}
              />{' '}
              启用 TE-Speed 包装节点（需确认本机插件兼容；默认使用原生采样）
            </label>
          </fieldset>
          <p className="settings-note">
            此模板原生 24 fps；重叠帧采用节点支持的选项。手动分段不受建议时长限制。Agent
            自动拆分与自动合并尚未启用。
          </p>
          <button className="outline-button" disabled={busy} type="submit">
            <Save size={15} />
            {busy ? '保存中…' : '保存长视频设置'}
          </button>
        </form>
      )}
      {error && (
        <p className="form-error" role="alert">
          {error}
        </p>
      )}
      {message && (
        <p className="form-success" role="status">
          {message}
        </p>
      )}
    </section>
  );
}
