import { ArrowUpRight, Camera, Clapperboard, Leaf, WandSparkles } from 'lucide-react';

const ideas = [
  {
    title: '山野来信',
    category: '自然纪实',
    theme: 'landscape',
    icon: Leaf,
    prompt:
      '清晨，薄雾沿着青绿色的山谷缓缓流动，阳光穿透云层，镜头平稳向前推进，远处的山峰逐渐显现。自然纪录片质感，柔和光线，保持画面稳定。',
  },
  {
    title: '光的形状',
    category: '产品广告',
    theme: 'product',
    icon: Camera,
    prompt:
      '一瓶透明香水置于浅绿色石台上，水面的反光轻轻掠过瓶身，镜头缓慢环绕，展现玻璃折射与细腻纹理。极简产品广告，自然侧光，无文字。',
  },
  {
    title: '落日之后',
    category: '电影叙事',
    theme: 'sunset',
    icon: Clapperboard,
    prompt:
      '落日时分，一位旅人站在沙丘的脊线上，微风拂动衣角，镜头从远景缓缓推近，夕阳将沙漠染成温暖的琥珀色。电影宽银幕构图，轻微胶片颗粒。',
  },
];
export function Inspiration({
  onChoose,
  disabled,
}: {
  onChoose: (prompt: string) => void;
  disabled: boolean;
}) {
  return (
    <section className="inspiration">
      <div className="section-heading">
        <div>
          <span className="eyebrow">A LITTLE INSPIRATION</span>
          <h2>
            灵感，从这里开始 <WandSparkles size={18} />
          </h2>
        </div>
        <span className="muted">选择一个方向，写下你的版本</span>
      </div>
      <div className="idea-grid">
        {ideas.map((idea, index) => (
          <button
            className={`idea-card ${idea.theme}`}
            key={idea.title}
            disabled={disabled}
            onClick={() => onChoose(idea.prompt)}
            aria-label={`使用灵感：${idea.title}`}
          >
            <div className="idea-art" aria-hidden="true">
              <span className="art-sun" />
              <span className="art-layer layer-back" />
              <span className="art-layer layer-front" />
              {idea.theme === 'product' && (
                <span className="bottle">
                  <i />
                </span>
              )}
            </div>
            <span className="idea-top">
              <span>
                <idea.icon size={13} />
                {idea.category}
              </span>
              <span>0{index + 1}</span>
            </span>
            <span className="idea-bottom">
              <span>
                <small>PROMPT STARTER</small>
                <strong>{idea.title}</strong>
              </span>
              <span className="idea-arrow">
                <ArrowUpRight size={20} />
              </span>
            </span>
          </button>
        ))}
      </div>
      <p className="art-caption">风格示意插画 · 点击填入提示词，不会自动提交生成</p>
    </section>
  );
}
