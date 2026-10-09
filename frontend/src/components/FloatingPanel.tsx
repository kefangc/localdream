import { useLayoutEffect, useRef, useState, type ReactNode } from 'react';
import { createPortal } from 'react-dom';

export type Anchor = { left: number; top: number; bottom: number };
export function FloatingPanel({
  anchor,
  children,
  className = '',
  onClose,
  label,
  placement = 'auto',
}: {
  anchor: () => Anchor;
  children: ReactNode;
  className?: string;
  onClose: () => void;
  label: string;
  placement?: 'auto' | 'bottom';
}) {
  const panel = useRef<HTMLDivElement>(null);
  const [position, setPosition] = useState({
    left: 0,
    top: 0,
    visibility: 'hidden' as 'hidden' | 'visible',
    maxHeight: window.innerHeight - 16,
  });
  useLayoutEffect(() => {
    const update = () => {
      const element = panel.current;
      if (!element) return;
      const point = anchor();
      const width = element.offsetWidth;
      const height = element.offsetHeight;
      // Prefer below-right; flip upward or shift left only to avoid the viewport edge.
      const left = Math.max(8, Math.min(point.left + 2, window.innerWidth - width - 8));
      const top =
        placement === 'bottom' || point.bottom + 6 + height <= window.innerHeight - 8
          ? point.bottom + 6
          : Math.max(8, point.top - height - 6);
      // A forced-bottom panel scrolls internally instead of flipping above its trigger.
      const maxHeight =
        placement === 'bottom'
          ? Math.max(40, window.innerHeight - top - 8)
          : window.innerHeight - 16;
      setPosition({ left, top, maxHeight, visibility: 'visible' });
    };
    update();
    const resize = new ResizeObserver(update);
    if (panel.current) resize.observe(panel.current);
    window.addEventListener('resize', update);
    window.addEventListener('scroll', update, true);
    const dismiss = (event: PointerEvent) => {
      if (
        !panel.current?.contains(event.target as Node) &&
        !(event.target as Element).closest('[data-floating-trigger]')
      )
        onClose();
    };
    const escape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose();
    };
    document.addEventListener('pointerdown', dismiss);
    document.addEventListener('keydown', escape);
    return () => {
      resize.disconnect();
      window.removeEventListener('resize', update);
      window.removeEventListener('scroll', update, true);
      document.removeEventListener('pointerdown', dismiss);
      document.removeEventListener('keydown', escape);
    };
  }, [anchor, onClose, placement]);
  return createPortal(
    <div ref={panel} className={`floating-panel ${className}`} style={position} aria-label={label}>
      {children}
    </div>,
    document.body,
  );
}
