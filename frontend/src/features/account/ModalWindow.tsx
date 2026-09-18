import { useEffect, useRef, type MouseEvent, type ReactNode } from 'react';
import { CLOSE_ACTION } from './accountCopy';

// One window is on screen at a time, so the heading it is labelled by is declared once.
const WINDOW_TITLE_ID = 'entry-window-title';

// What a window puts the cursor on when it opens: the first field of its content. A person opens a
// form to type in it, so the cursor does not go to whichever control comes first — the button that
// closes the window is one of those.
const FIRST_FIELD = 'input, select, textarea';

type ModalWindowProps = {
  open: boolean;
  title: string;
  onClose: () => void;
  children: ReactNode;
};

/**
 * ModalWindow is how the application shows a window over the page. It is a native dialog opened
 * modally, so the layer above the page, the focus kept inside it and closing on Esc are the
 * browser's rather than ours, and it knows nothing about what it holds beyond the title it carries.
 *
 * The element stays in the document while the window is closed, because the browser returns the
 * focus to whatever opened the window when it closes: a window that unmounted would leave the person
 * at the top of the page looking for where they were.
 */
export function ModalWindow({ open, title, onClose, children }: ModalWindowProps) {
  const window = useRef<HTMLDialogElement>(null);
  const content = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const element = window.current;
    if (element === null) return;

    if (open && !element.open) {
      element.showModal();
      content.current?.querySelector<HTMLElement>(FIRST_FIELD)?.focus();
    }

    if (!open && element.open) element.close();
  }, [open]);

  // The backdrop is the dialog itself: a click on it has the dialog as its target, while a click
  // inside the window has one of the parts the window is built from.
  function closesOnBackdrop(event: MouseEvent<HTMLDialogElement>) {
    if (event.target === event.currentTarget) onClose();
  }

  return (
    <dialog
      className="entry-window"
      ref={window}
      aria-labelledby={WINDOW_TITLE_ID}
      onCancel={onClose}
      onClick={closesOnBackdrop}
    >
      <div className="entry-header">
        <h2 className="entry-title" id={WINDOW_TITLE_ID}>
          {title}
        </h2>
        <button className="entry-close" type="button" aria-label={CLOSE_ACTION} onClick={onClose}>
          ×
        </button>
      </div>
      {/* A closed window holds nothing either: what was typed into it does not stay behind the page. */}
      <div className="entry-content" ref={content}>
        {open && children}
      </div>
    </dialog>
  );
}
