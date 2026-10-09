import { useEffect, useRef, useState } from 'react';
import { Bot, Check, RefreshCw, Save, X } from 'lucide-react';
import { api } from '../lib/api';
import type { Provider, LongVideoSettings } from '../types';
import { LongVideoSettingsPanel } from './LongVideoSettingsPanel';

export function SettingsDialog({
  onClose,
  onLongVideoSettingsSaved,
}: {
  onClose: () => void;
  onLongVideoSettingsSaved?: (settings: LongVideoSettings) => void;
}) {
  const dialog = useRef<HTMLDialogElement>(null);
  const [provider, setProvider] = useState<Provider>({ base_url: '', api_key: '', model: '' });
  const [models, setModels] = useState<string[]>([]);
  const [busy, setBusy] = useState('loading');
  const [message, setMessage] = useState('');
  const [error, setError] = useState('');
  useEffect(() => {
    // Native <dialog> provides focus trapping, Escape handling, and focus restoration.
    dialog.current?.showModal();
    let disposed = false;
    api
      .provider()
      .then((data) => {
        if (!disposed)
          setProvider({ base_url: data.base_url || '', model: data.model || '', api_key: '' });
      })
      .catch((reason) => {
        if (!disposed) setError(reason.message);
      })
      .finally(() => {
        if (!disposed) setBusy('');
      });
    return () => {
      disposed = true;
    };
  }, []);
  async function perform(action: 'save' | 'models') {
    setBusy(action);
    setError('');
    setMessage('');
    try {
      if (action === 'models') {
        const result = await api.models(provider);
        setModels(Array.isArray(result.models) ? result.models : []);
        setMessage(
          result.models?.length
            ? `找到 ${result.models.length} 个模型。`
            : '没有可用模型，可手动输入名称。',
        );
      } else {
        await api.saveProvider(provider);
        setProvider((value) => ({ ...value, api_key: '' }));
        setMessage('配置已保存到本机。');
      }
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '操作失败');
    } finally {
      setBusy('');
    }
  }
  return (
    <dialog
      ref={dialog}
      className="settings-dialog"
      aria-labelledby="settings-title"
      onCancel={(event) => {
        event.preventDefault();
        onClose();
      }}
    >
      <div className="dialog-heading">
        <span className="empty-symbol">
          <Bot size={23} />
        </span>
        <button className="icon-button" aria-label="关闭设置" onClick={onClose}>
          <X size={20} />
        </button>
      </div>
      <h2 id="settings-title">工作室设置</h2>
      <LongVideoSettingsPanel onSaved={onLongVideoSettingsSaved} />
      <h3>Agent 模型连接</h3>
      <p className="muted">连接 OpenAI 兼容模型，帮助你完善镜头与画面描述。</p>
      <form
        onSubmit={(event) => {
          event.preventDefault();
          void perform('save');
        }}
      >
        <fieldset disabled={!!busy} className="settings-fields">
          <label>
            Base URL
            <input
              type="url"
              required
              value={provider.base_url}
              placeholder="https://api.example.com/v1"
              onChange={(event) => setProvider({ ...provider, base_url: event.target.value })}
            />
          </label>
          <label>
            API Key
            <input
              type="password"
              autoComplete="off"
              value={provider.api_key}
              placeholder="留空保留已保存的密钥"
              onChange={(event) => setProvider({ ...provider, api_key: event.target.value })}
            />
          </label>
          <label>
            模型名称
            <input
              required
              list="provider-models"
              value={provider.model}
              placeholder="输入模型名称，或从列表选择"
              onChange={(event) => setProvider({ ...provider, model: event.target.value })}
            />
            <datalist id="provider-models">
              {models.map((model) => (
                <option key={model} value={model} />
              ))}
            </datalist>
          </label>
          <button
            type="button"
            className="outline-button"
            disabled={!provider.base_url}
            onClick={() => void perform('models')}
          >
            <RefreshCw size={15} />
            获取模型列表
          </button>
        </fieldset>
        {error && (
          <p className="form-error" role="alert">
            {error}
          </p>
        )}
        {message && (
          <p className="form-success" role="status">
            <Check size={15} />
            {message}
          </p>
        )}
        <p className="settings-note">
          密钥保存在本机服务配置中。使用 Agent 时，描述与素材信息会发往此处配置的服务。
        </p>
        <div className="dialog-footer">
          <button type="button" className="text-button" onClick={onClose}>
            关闭
          </button>
          <button type="submit" className="primary-button" disabled={!!busy}>
            <Save size={15} />
            {busy === 'save' ? '保存中…' : busy === 'loading' ? '加载中…' : '保存配置'}
          </button>
        </div>
      </form>
    </dialog>
  );
}
