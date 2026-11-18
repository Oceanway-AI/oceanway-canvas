import {
  useEffect,
  useState,
  type ButtonHTMLAttributes,
  type ReactNode,
  type Ref,
} from "react";
import { Popover } from "radix-ui";
import "./account-panel.css";

/** A separate account popover anchored to the sidebar footer. */
export function OceanAccountPopover(props: {
  trigger: ReactNode;
  children: ReactNode;
  label: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  portalContainer?: HTMLElement | null;
}) {
  const [narrow, setNarrow] = useState(false);
  useEffect(() => {
    const query = window.matchMedia("(max-width: 640px)");
    const update = () => setNarrow(query.matches);
    update();
    query.addEventListener("change", update);
    return () => query.removeEventListener("change", update);
  }, []);
  return (
    <Popover.Root open={props.open} onOpenChange={props.onOpenChange}>
      <Popover.Trigger asChild>{props.trigger}</Popover.Trigger>
      <Popover.Portal container={props.portalContainer}>
        <Popover.Content
          className="ow-shared-account-popover"
          aria-label={props.label}
          side={narrow ? "top" : "right"}
          align={narrow ? "start" : "end"}
          sideOffset={8}
          collisionPadding={8}
        >
          {props.children}
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  );
}

/** Pure shared layout. Hosts own identity, permissions, requests and navigation. */
export function OceanAccountPanel(props: {
  name: string;
  identity: ReactNode;
  avatar: ReactNode;
  children: ReactNode;
  tools: ReactNode;
  density?: "standard" | "compact";
}) {
  return (
    <div className="ow-shared-account-panel" data-density={props.density}>
      <header className="ow-shared-account-panel-identity">
        {props.avatar}
        <div>
          <strong>{props.name}</strong>
          <span>{props.identity}</span>
        </div>
      </header>
      <div className="ow-shared-account-panel-cards">{props.children}</div>
      <div className="ow-shared-account-panel-tools">{props.tools}</div>
    </div>
  );
}

/** Sidebar footer: personal identity and confirmed workspace, supplied by host. */
export function OceanAccountTrigger({
  name,
  workspace,
  avatar,
  className,
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & {
  name: string;
  workspace: string;
  avatar: ReactNode;
  ref?: Ref<HTMLButtonElement>;
}) {
  return (
    <button
      type="button"
      aria-label={`${name} · ${workspace}`}
      {...props}
      className={["ow-shared-account-trigger", className]
        .filter(Boolean)
        .join(" ")}
    >
      {avatar}
      <span className="ow-shared-account-trigger-copy">
        <strong title={name}>{name}</strong>
        <span title={workspace}>{workspace}</span>
      </span>
      <span className="ow-shared-account-trigger-chevron" aria-hidden="true">
        ⌃
      </span>
    </button>
  );
}

export function OceanMembershipCard(props: {
  mark: ReactNode;
  title: ReactNode;
  scope: ReactNode;
  action?: ReactNode;
  links?: ReactNode;
}) {
  return (
    <section className="ow-shared-account-membership">
      <div className="ow-shared-account-membership-main">
        <div className="ow-shared-account-membership-copy">
          <strong>
            {props.mark}
            <span>{props.title}</span>
          </strong>
          <span>{props.scope}</span>
        </div>
        {props.action}
      </div>
      {props.links && (
        <div className="ow-shared-account-membership-links">{props.links}</div>
      )}
    </section>
  );
}

export function OceanCreditsCard(props: {
  label: string;
  amount: ReactNode;
  unit?: ReactNode;
  details?: ReactNode;
  children?: ReactNode;
}) {
  return (
    <section className="ow-shared-account-credits" aria-label={props.label}>
      <h3>{props.label}</h3>
      <div className="ow-shared-account-credits-card">
        <div className="ow-shared-account-credits-main">
          <strong>
            {props.amount}
            {props.unit && (
              <small className="ow-shared-account-unit">{props.unit}</small>
            )}
          </strong>
          {props.details}
        </div>
        {props.children && (
          <div className="ow-shared-account-credits-secondary">
            {props.children}
          </div>
        )}
      </div>
    </section>
  );
}

export function OceanAccountAction({
  icon,
  trailing,
  children,
  className,
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & {
  icon?: ReactNode;
  trailing?: ReactNode;
}) {
  return (
    <button
      type="button"
      {...props}
      className={["ow-shared-account-action", className]
        .filter(Boolean)
        .join(" ")}
    >
      {icon}
      <span>{children}</span>
      {trailing && (
        <span className="ow-shared-account-action-trailing" aria-hidden="true">
          {trailing}
        </span>
      )}
    </button>
  );
}
