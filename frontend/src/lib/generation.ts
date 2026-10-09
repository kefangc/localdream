import type { Asset, GenerationOptions, Mode } from '../types';

export const modes: { id: Mode; label: string; hint: string }[] = [
  { id: 'text_to_video', label: '文生视频', hint: '从一句描述开始，让想象自由发生。' },
  { id: 'image_to_video', label: '图生视频', hint: '添加并引用一张图片，赋予静止画面新的生命。' },
  { id: 'first_last_frame', label: '首尾帧', hint: '依次引用起始帧与结束帧，连接两个瞬间。' },
  {
    id: 'multi_reference_video',
    label: '多参考视频',
    hint: '用 @ 按顺序引用图片、视频或音频，组合你的故事。',
  },
];
export const ratios = ['16:9', '9:16', '1:1', '4:3'];
export const resolutions = ['480p', '720p'];
export const defaults: GenerationOptions = {
  reference_video_fps: 7,
  mode: 'text_to_video',
  duration: 5,
  aspect_ratio: '16:9',
  resolution: '720p',
  steps: 50,
  lora: '',
  lora_strength: 0.75,
  use_agent: false,
};
type Acceleration = { value: string; label: string; steps?: number };
const standard: Acceleration = { value: '', label: '标准 · 自定义步数' };
const fl2v: Acceleration[] = [
  standard,
  { value: 'fl2v_4', label: 'FL2V · 4 步', steps: 4 },
  { value: 'fl2v_8', label: 'FL2V · 8 步', steps: 8 },
];
const ref2v: Acceleration[] = [
  standard,
  { value: 'ref2v_4', label: 'Ref2V · 4 步', steps: 4 },
  { value: 'ref2v_8', label: 'Ref2V · 8 步', steps: 8 },
];
export const accelerationByMode: Record<Mode, Acceleration[]> = {
  text_to_video: fl2v,
  image_to_video: ref2v,
  first_last_frame: fl2v,
  multi_reference_video: ref2v,
};
export const modeLabel = (mode: string) => modes.find((item) => item.id === mode)?.label ?? mode;
export const isActiveJob = (status: string) => status === 'queued' || status === 'running';

// Match explicit @mentions, longest name first, to avoid matching "a.png" inside "a.png.png".
// Preview and payload share this pure function, so their asset ordering stays consistent.
export function referencedAssets(prompt: string, assets: Asset[]): Asset[] {
  const names = [...assets].sort((a, b) => b.name.length - a.name.length);
  const result: Asset[] = [];
  for (let position = 0; position < prompt.length; position++) {
    if (prompt[position] !== '@') continue;
    const match = names.find((asset) => prompt.startsWith(asset.name, position + 1));
    if (match) {
      if (!result.some((asset) => asset.id === match.id)) result.push(match);
      position += match.name.length;
    }
  }
  return result;
}
export function changeMode(options: GenerationOptions, mode: Mode): GenerationOptions {
  // LoRAs belong to a model family; do not carry one into an incompatible mode.
  return { ...options, mode, lora: '' };
}
export function validateGeneration(prompt: string, options: GenerationOptions, assets: Asset[]) {
  const fps = options.reference_video_fps;
  if (!Number.isFinite(fps) || (fps !== 0 && (fps < 1 || fps > 60)))
    return '参考视频 FPS 须为 1–60，或设为 0 使用源帧率。';
  if (!prompt.trim()) return '先描述你想创作的画面。';
  const references = referencedAssets(prompt, assets);
  const images = references.filter((asset) => asset.type === 'image');
  if (options.mode === 'image_to_video' && images.length === 0) return '请先用 @ 引用一张图片。';
  if (options.mode === 'first_last_frame' && images.length < 2)
    return '请按顺序引用起始帧和结束帧两张图片。';
  if (options.mode === 'multi_reference_video' && references.length === 0)
    return '请至少引用一个参考素材。';
  return null;
}
