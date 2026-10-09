import { useEffect, useRef, useState, type ReactNode } from 'react';
import { MoreHorizontal } from 'lucide-react';
import { FloatingPanel } from './FloatingPanel';

export type MenuItem = {
  label: string;
  icon: ReactNode;
  onSelect: () => void;
  danger?: boolean;
  disabled?: boolean;
};
export function ActionMenu({
  label,
  items,
  children,
  disabled = false,
}: {
  label: string;
  items: MenuItem[];
  children?: ReactNode;
  disabled?: boolean;
}) {
  const trigger = useRef<HTMLButtonElement>(null);
  const menu = useRef<HTMLDivElement>(null);
  const [open, setOpen] = useState(false);
  useEffect(() => {
    if (open) menu.current?.querySelector<HTMLButtonElement>('button:not(:disabled)')?.focus();
  }, [open]);
  function close() {
    setOpen(false);
  }
  return (
    <>
      <button
        ref={trigger}
        type="button"
        className={`icon-button menu-trigger ${open ? 'open' : ''}`}
        data-floating-trigger
        disabled={disabled}
        aria-label={label}
        aria-haspopup="menu"
        aria-expanded={open}
        onClick={() => setOpen(!open)}
      >
        <MoreHorizontal size={18} />
      </button>
      {open && (
        <FloatingPanel
          label={label}
          className="action-menu"
          anchor={() => trigger.current!.getBoundingClientRect()}
          onClose={close}
        >
          <div
            ref={menu}
            role="menu"
            aria-label={label}
            onKeyDown={(e) => {
              if (e.key === 'Escape') {
                close();
                trigger.current?.focus();
              }
              if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
                e.preventDefault();
                const buttons = Array.from(
                  menu.current!.querySelectorAll<HTMLButtonElement>(
                    'button[role="menuitem"]:not(:disabled)',
                  ),
                );
                const index = buttons.indexOf(document.activeElement as HTMLButtonElement);
                buttons[
                  (index + (e.key === 'ArrowDown' ? 1 : -1) + buttons.length) % buttons.length
                ]?.focus();
              }
            }}
          >
            {items.map((item) => (
              <button
                type="button"
                role="menuitem"
                key={item.label}
                className={item.danger ? 'danger' : ''}
                disabled={item.disabled}
                onClick={() => {
                  close();
                  item.onSelect();
                }}
              >
                {item.icon}
                <span>{item.label}</span>
              </button>
            ))}
          </div>
          {children && <div className="action-menu-extra">{children}</div>}
        </FloatingPanel>
      )}
    </>
  );
}
