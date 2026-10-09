import { Film, ImagePlus, Layers3, Sparkles, Video } from 'lucide-react';
import { modes } from '../lib/generation';
import type { Mode } from '../types';

export function CreationModes({
  mode,
  long,
  onSelect,
  disabled,
}: {
  mode: Mode;
  long: boolean;
  onSelect: (mode: Mode | 'long') => void;
  disabled: boolean;
}) {
  return (
    <div className="mode-tabs creation-modes" role="group" aria-label="创作模式">
      {modes.map((item) => {
        const Icon =
          item.id === 'text_to_video'
            ? Sparkles
            : item.id === 'image_to_video'
              ? ImagePlus
              : item.id === 'first_last_frame'
                ? Film
                : Video;
        return (
          <button
            type="button"
            key={item.id}
            disabled={disabled}
            aria-pressed={!long && mode === item.id}
            className={!long && mode === item.id ? 'selected' : ''}
            onClick={() => onSelect(item.id)}
          >
            <Icon size={16} />
            {item.label}
          </button>
        );
      })}
      <button
        type="button"
        disabled={disabled}
        className={long ? 'selected' : ''}
        aria-pressed={long}
        onClick={() => onSelect('long')}
      >
        <Layers3 size={16} />
        长视频
      </button>
    </div>
  );
}
