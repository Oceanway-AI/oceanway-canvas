import {
  useEffect,
  useId,
  useRef,
  useState,
  type CSSProperties,
  type ReactNode,
} from "react";
import "./context-switcher.css";
import { Popover } from "radix-ui";

/** The host supplies the same team avatar used by the workspace list. */
export function OceanWorkspaceAvatar({
  avatar,
  teamAvatar,
  size = 28,
}: {
  avatar: ReactNode;
  teamAvatar?: ReactNode;
  size?: number;
}) {
  return (
    <span
      className="ow-workspace-avatar"
      style={{ "--ow-avatar-size": `${size}px` } as CSSProperties}
      aria-hidden="true"
    >
      <span className="ow-workspace-avatar-person">{avatar}</span>
      {teamAvatar && (
        <span className="ow-workspace-avatar-team">{teamAvatar}</span>
      )}
    </span>
  );
}

export type OceanContextItem = {
  id: string;
  label: string;
  icon?: ReactNode;
  description?: string;
  disabled?: boolean;
};

export type OceanContextSwitcherProps = {
  /** Supplied by the host after checking its current account and workspace. */
  current: OceanContextItem;
  items: readonly OceanContextItem[];
  label: string;
  currentLabel: string;
  loadingLabel: string;
  emptyLabel: string;
  errorLabel: string;
  retryLabel: string;
  pendingLabel: string;
  state?: "ready" | "loading" | "error";
  error?: string;
  pendingId?: string;
  disabled?: boolean;
  compact?: boolean;
  defaultOpen?: boolean;
  className?: string;
  onSelect: (id: string) => void;
  onRetry?: () => void;
  /** A host theme root, outside any scrolling account panel. */
  portalContainer?: HTMLElement | null;
};

/** Product navigation and workspace selection share presentation, never state. */
function ContextSwitcher({
  current,
  items,
  label,
  currentLabel,
  loadingLabel,
  emptyLabel,
  errorLabel,
  retryLabel,
  pendingLabel,
  state = "ready",
  error,
  pendingId,
  disabled,
  compact,
  defaultOpen = false,
  className = "",
  onSelect,
  onRetry,
  portalContainer,
  side,
}: OceanContextSwitcherProps & { side: "bottom" | "right" }) {
  const [open, setOpen] = useState(defaultOpen);
  const [narrow, setNarrow] = useState(false);
  useEffect(() => {
    const query = window.matchMedia("(max-width: 640px)");
    const update = () => setNarrow(query.matches);
    update();
    query.addEventListener("change", update);
    return () => query.removeEventListener("change", update);
  }, []);
  const root = useRef<HTMLDivElement>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const panel = useRef<HTMLDivElement>(null);
  const previous = useRef(current.id);
  const id = useId();
  const visible = open && !disabled;
  function close(restoreFocus = false) {
    setOpen(false);
    if (restoreFocus) trigger.current?.focus();
  }

  useEffect(() => {
    if (disabled) setOpen(false);
  }, [disabled]);

  useEffect(() => {
    if (previous.current !== current.id) {
      previous.current = current.id;
      setOpen(false);
      if (root.current?.contains(document.activeElement))
        trigger.current?.focus();
    }
  }, [current.id]);

  return (
    <Popover.Root open={visible} onOpenChange={setOpen}>
      <div
        ref={root}
        className={`ow-context-switcher ${className}`}
        data-side={side}
        data-compact={compact || undefined}
      >
        <Popover.Trigger asChild>
          <button
            ref={trigger}
            type="button"
            className="ow-context-trigger"
            aria-label={`${label}：${current.label}`}
            aria-expanded={visible}
            aria-controls={visible ? id : undefined}
            disabled={disabled}
            onKeyDown={(event) => {
              if (visible && event.key === "ArrowDown") {
                event.preventDefault();
                panel.current
                  ?.querySelector<HTMLButtonElement>("button:not(:disabled)")
                  ?.focus();
              }
            }}
          >
            <span className="ow-context-icon" aria-hidden="true">
              {current.icon}
            </span>
            <span className="ow-context-label">{current.label}</span>
            <svg
              className="ow-context-chevron"
              aria-hidden="true"
              viewBox="0 0 16 16"
              fill="none"
            >
              <path
                d="m4 6 4 4 4-4"
                stroke="currentColor"
                strokeWidth="1.5"
                strokeLinecap="round"
                strokeLinejoin="round"
              />
            </svg>
          </button>
        </Popover.Trigger>
        <ContextPortal container={portalContainer}>
          <Popover.Content
            id={id}
            ref={panel}
            className="ow-context-panel"
            side={narrow && side === "right" ? "top" : side}
            align={side === "right" ? "end" : "start"}
            sideOffset={side === "right" && !narrow ? 22 : 6}
            collisionPadding={8}
            data-side-menu={side === "right" || undefined}
            data-compact={compact || undefined}
            onOpenAutoFocus={(event) => event.preventDefault()}
            onEscapeKeyDown={(event) => event.stopPropagation()}
            aria-label={label}
            aria-busy={state === "loading" || Boolean(pendingId)}
          >
            {state === "loading" ? (
              <p className="ow-context-status" role="status">
                {loadingLabel}
              </p>
            ) : state === "error" ? (
              <div className="ow-context-status">
                <p role="alert">{error || errorLabel}</p>
                {onRetry && (
                  <button
                    type="button"
                    className="ow-context-retry"
                    onClick={() => {
                      trigger.current?.focus();
                      onRetry();
                    }}
                  >
                    {retryLabel}
                  </button>
                )}
              </div>
            ) : (
              <>
                {items.length === 0 ? (
                  <p className="ow-context-status" role="status">
                    {emptyLabel}
                  </p>
                ) : (
                  <ul className="ow-context-list">
                    {items.map((item) => {
                      const selected = item.id === current.id;
                      const pending = item.id === pendingId;
                      return (
                        <li key={item.id}>
                          <button
                            type="button"
                            className="ow-context-option"
                            aria-current={selected ? "true" : undefined}
                            disabled={item.disabled || Boolean(pendingId)}
                            onClick={() => {
                              if (selected) close(true);
                              else {
                                trigger.current?.focus();
                                onSelect(item.id);
                              }
                            }}
                          >
                            <span
                              className="ow-context-icon"
                              aria-hidden="true"
                            >
                              {item.icon}
                            </span>
                            <span className="ow-context-copy">
                              <span>{item.label}</span>
                              {item.description && (
                                <small>{item.description}</small>
                              )}
                            </span>
                            {selected && (
                              <span
                                className="ow-context-check"
                                aria-label={currentLabel}
                              >
                                ✓
                              </span>
                            )}
                            {pending && (
                              <span
                                className="ow-context-pending"
                                role="status"
                              >
                                {pendingLabel}
                              </span>
                            )}
                          </button>
                        </li>
                      );
                    })}
                  </ul>
                )}
                {error && (
                  <p className="ow-context-status" role="alert">
                    {error}
                  </p>
                )}
              </>
            )}
          </Popover.Content>
        </ContextPortal>
      </div>
    </Popover.Root>
  );
}

/** C01: an anchored list for the registered product catalog. */
export function OceanProductSwitcher(props: OceanContextSwitcherProps) {
  return <ContextSwitcher {...props} side="bottom" />;
}

/** Workspace options open beside the account panel without changing its height. */
export function OceanWorkspaceSwitcher(props: OceanContextSwitcherProps) {
  return <ContextSwitcher {...props} side="right" />;
}

function ContextPortal({
  container,
  children,
}: {
  container?: HTMLElement | null;
  children: ReactNode;
}) {
  return container ? (
    <Popover.Portal container={container}>{children}</Popover.Portal>
  ) : (
    children
  );
}
