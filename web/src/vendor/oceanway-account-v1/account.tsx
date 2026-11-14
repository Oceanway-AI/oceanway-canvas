import {
  type CSSProperties,
  type MouseEvent,
  type ReactNode,
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import { createPortal } from "react-dom";
import "./account.css";

export type AccountBalance =
  | { state: "ready"; amount: string }
  | { state: "loading" | "restricted" | "unavailable" };

export interface AccountLabels {
  navigation: string;
  products: string;
  spaces: string;
  account: string;
  wallet: string;
  balance: string;
  credits: string;
  loading: string;
  restricted: string;
  unavailable: string;
  retry: string;
}

const defaultLabels: AccountLabels = {
  navigation: "OceanWay 账户与工作空间",
  products: "切换产品",
  spaces: "工作空间",
  account: "账户中心",
  wallet: "钱包与使用记录",
  balance: "OceanWay 通用余额",
  credits: "OceanWay 积分",
  loading: "正在读取",
  restricted: "无查看权限",
  unavailable: "暂时无法读取",
  retry: "重试",
};

export interface OceanAccountBarProps {
  productName: string;
  products: readonly { name: string; href: string }[];
  workspaces: readonly { id: string; name: string }[];
  workspaceId?: string;
  onWorkspaceChange?: (id: string) => void;
  displayName: string;
  avatarUrl?: string;
  balance: AccountBalance;
  balanceLabel?: string;
  walletHref?: string;
  accountHref?: string;
  linkTarget?: "_self" | "_blank";
  onRetry?: () => void;
  compact?: boolean;
  accountDetails?: ReactNode;
  labels?: Partial<AccountLabels>;
}

export function OceanCreditAmount({
  amount,
  unitLabel = defaultLabels.credits,
  className,
}: {
  amount: string;
  unitLabel?: string;
  className?: string;
}) {
  return (
    <span className={["ow-credit-amount", className].filter(Boolean).join(" ")}>
      <span aria-hidden="true">✦ {amount}</span>
      <span className="ow-account-sr">
        {amount} {unitLabel}
      </span>
    </span>
  );
}

export function OceanAvatar({
  displayName,
  avatarUrl,
  className,
  size = 32,
}: {
  displayName: string;
  avatarUrl?: string;
  className?: string;
  size?: number;
}) {
  const [failedUrl, setFailedUrl] = useState<string>();
  return (
    <span
      className={["ow-account-avatar", className].filter(Boolean).join(" ")}
      style={{ "--ow-avatar-size": `${size}px` } as CSSProperties}
      role="img"
      aria-label={displayName}
    >
      {avatarUrl && failedUrl !== avatarUrl ? (
        <img
          key={avatarUrl}
          src={avatarUrl}
          alt=""
          referrerPolicy="no-referrer"
          onError={() => setFailedUrl(avatarUrl)}
        />
      ) : (
        <span aria-hidden="true">
          {[...displayName.trim()][0]?.toLocaleUpperCase() || "•"}
        </span>
      )}
    </span>
  );
}

type Menu = "products" | "spaces" | "account";
const focusable =
  'a[href], button:not(:disabled), input:not(:disabled), select:not(:disabled), [tabindex="0"]';

export function OceanAccountBar({
  productName,
  products,
  workspaces,
  workspaceId,
  onWorkspaceChange,
  displayName,
  avatarUrl,
  balance,
  balanceLabel,
  walletHref,
  accountHref,
  linkTarget,
  onRetry,
  compact = false,
  accountDetails,
  labels: overrides,
}: OceanAccountBarProps) {
  const labels = { ...defaultLabels, ...overrides };
  const title = balanceLabel || labels.balance;
  const currentSpace = workspaces.find((space) => space.id === workspaceId);
  const trigger = useRef<HTMLButtonElement | null>(null);
  const panel = useRef<HTMLDivElement | null>(null);
  const bar = useRef<HTMLElement | null>(null);
  const panelId = useId();
  const [menu, setMenu] = useState<Menu>();
  const [position, setPosition] = useState<CSSProperties>({});
  const credit =
    balance.state === "ready" ? (
      <OceanCreditAmount amount={balance.amount} unitLabel={labels.credits} />
    ) : (
      labels[balance.state]
    );

  function close(focusTrigger: boolean) {
    setMenu(undefined);
    if (focusTrigger) trigger.current?.focus();
  }

  function toggle(next: Menu, button: HTMLButtonElement) {
    trigger.current = button;
    setMenu(menu === next ? undefined : next);
  }

  useLayoutEffect(() => {
    if (!menu) return;
    const button = trigger.current;
    const floating = panel.current;
    if (!button || !floating) return;
    function place() {
      if (!button || !floating) return;
      const rect = button.getBoundingClientRect();
      const gap = 6,
        margin = 8;
      const below = window.innerHeight - rect.bottom - gap - margin;
      const above = rect.top - gap - margin;
      const left = Math.max(
        margin,
        Math.min(
          rect.right - floating.offsetWidth,
          window.innerWidth - floating.offsetWidth - margin,
        ),
      );
      const bottomSide = below >= floating.scrollHeight || below >= above;
      setPosition(
        bottomSide
          ? { left, top: rect.bottom + gap, maxHeight: Math.max(0, below) }
          : {
              left,
              bottom: window.innerHeight - rect.top + gap,
              maxHeight: Math.max(0, above),
            },
      );
    }
    place();
    (floating.querySelector<HTMLElement>(focusable) || floating).focus({
      preventScroll: true,
    });
    window.addEventListener("resize", place);
    window.addEventListener("scroll", place, true);
    return () => {
      window.removeEventListener("resize", place);
      window.removeEventListener("scroll", place, true);
    };
  }, [menu]);

  useEffect(() => {
    if (!menu) return;
    function dismiss(event: Event) {
      if (
        event.target instanceof Node &&
        !panel.current?.contains(event.target) &&
        !bar.current?.contains(event.target)
      ) {
        close(false);
      }
    }
    function escape(event: KeyboardEvent) {
      if (event.key === "Escape") {
        event.stopPropagation();
        close(true);
      }
    }
    document.addEventListener("pointerdown", dismiss, true);
    document.addEventListener("focusin", dismiss);
    document.addEventListener("keydown", escape, true);
    return () => {
      document.removeEventListener("pointerdown", dismiss, true);
      document.removeEventListener("focusin", dismiss);
      document.removeEventListener("keydown", escape, true);
    };
  }, [menu]);

  const link = {
    target: linkTarget,
    rel: linkTarget === "_blank" ? "noopener noreferrer" : undefined,
  };
  const triggerProps = (key: Menu) => ({
    "aria-expanded": key === menu,
    "aria-controls": key === menu ? panelId : undefined,
    onClick: (event: MouseEvent<HTMLButtonElement>) =>
      toggle(key, event.currentTarget),
  });
  return (
    <nav
      ref={bar}
      className="ow-account-bar"
      data-compact={compact || undefined}
      aria-label={labels.navigation}
    >
      <button
        type="button"
        className="ow-account-control"
        aria-label={`${labels.products} · OceanWay ${productName}`}
        {...triggerProps("products")}
      >
        <span className="ow-account-brand" aria-hidden="true" />
        <span className="ow-account-name">{productName}</span>
      </button>
      {currentSpace && (
        <button
          type="button"
          className="ow-account-control ow-account-space"
          disabled={!onWorkspaceChange || workspaces.length <= 1}
          aria-label={`${labels.spaces} · ${currentSpace.name}`}
          title={currentSpace.name}
          {...triggerProps("spaces")}
        >
          <span>{currentSpace.name}</span>
        </button>
      )}
      {walletHref ? (
        <a
          className="ow-account-control"
          href={walletHref}
          {...link}
          title={title}
          aria-label={`${title} · ${balance.state === "ready" ? `${balance.amount} ${labels.credits}` : labels[balance.state]}`}
        >
          {credit}
        </a>
      ) : (
        <span className="ow-account-control" title={title}>
          {credit}
        </span>
      )}
      <button
        type="button"
        className="ow-account-user"
        aria-label={`${labels.account} · ${displayName}`}
        {...triggerProps("account")}
      >
        <OceanAvatar displayName={displayName} avatarUrl={avatarUrl} />
      </button>
      {menu &&
        createPortal(
          <div
            ref={panel}
            tabIndex={-1}
            id={panelId}
            className="ow-account-panel"
            role="group"
            aria-label={labels[menu]}
            style={position}
            onKeyDown={(event) => {
              if (event.key !== "Tab" || !panel.current) return;
              const elements =
                panel.current.querySelectorAll<HTMLElement>(focusable);
              if (
                (!event.shiftKey &&
                  event.target === elements.item(elements.length - 1)) ||
                (event.shiftKey && event.target === elements[0]) ||
                elements.length === 0
              ) {
                // Resume normal tab order at the trigger rather than at the end of the portal.
                close(true);
              }
            }}
          >
            {menu === "products" && (
              <>
                <strong>{labels.products}</strong>
                {products.map((product) => (
                  <a
                    key={product.href}
                    href={product.href}
                    aria-current={
                      product.name === productName ? "page" : undefined
                    }
                  >
                    {product.name}
                  </a>
                ))}
              </>
            )}
            {menu === "spaces" && (
              <>
                <strong>{labels.spaces}</strong>
                {workspaces.map((space) => (
                  <button
                    type="button"
                    key={space.id}
                    aria-current={workspaceId === space.id || undefined}
                    onClick={() => {
                      close(true);
                      if (space.id !== workspaceId) {
                        onWorkspaceChange?.(space.id);
                      }
                    }}
                  >
                    {space.name}
                  </button>
                ))}
              </>
            )}
            {menu === "account" && (
              <>
                <header>
                  <OceanAvatar
                    displayName={displayName}
                    avatarUrl={avatarUrl}
                  />
                  <strong>{displayName}</strong>
                </header>
                {currentSpace && (
                  <div className="ow-account-row">
                    <span>{labels.spaces}</span>
                    <strong>{currentSpace.name}</strong>
                  </div>
                )}
                <div className="ow-account-row">
                  <span>{title}</span>
                  <strong>{credit}</strong>
                </div>
                {balance.state === "unavailable" && onRetry && (
                  <button type="button" onClick={onRetry}>
                    {labels.retry}
                  </button>
                )}
                {walletHref && (
                  <a href={walletHref} {...link}>
                    {labels.wallet}
                  </a>
                )}
                {accountDetails}
                {accountHref && (
                  <a href={accountHref} {...link}>
                    {labels.account}
                  </a>
                )}
              </>
            )}
          </div>,
          document.body,
        )}
    </nav>
  );
}
