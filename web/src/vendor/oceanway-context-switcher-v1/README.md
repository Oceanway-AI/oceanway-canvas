# Product and workspace switching

`OceanProductSwitcher` renders an anchored product list. `OceanWorkspaceSwitcher`
opens a separate side popup from the account panel. Both require React 19 and radix-ui 1.6.7 and import the
adjacent stylesheet. Copy only a packaged artifact with its immutable manifest;
do not modify the copy in a product repository.

The host supplies its authorized catalog, current item, translated labels and
callbacks. Icons use approved product or OceanWay assets supplied by the host.
Workspace rows show the workspace name and current selection, without owner names
or member roles. Use `description` only when an entry needs an availability reason.
Mount product switching at the top of the existing sidebar. The bottom sidebar
identity area shows the personal avatar, name and confirmed workspace only. Clicking
that identity area opens the separate compact account panel; its workspace row opens
a separate options popup without increasing panel height. Team management placement
is pending user discussion and is not presented as an accepted action here.
`OceanWorkspaceAvatar` composes the existing personal avatar with a small team
avatar at its bottom-right. Supply the identical team avatar to the corresponding
workspace item's `icon`. Omit `teamAvatar` in personal space. Hosts own uploaded
images or name-initial fallbacks, and must label the surrounding account button
with the current workspace; the composite is decorative beside that label.
The component never fetches data, stores a workspace, changes location or grants
permissions. Reading an error or loading state does not expose old list entries.

`onSelect` fires only for a different, enabled item. The current item closes the
list and restores focus without emitting a selection. For workspace confirmation,
set `pendingId` immediately, perform the host's explicit selection request, then
replace `current` only on success. On failure clear `pendingId`, keep the original
current item and provide `error`; the user may retry. Changing `current.id` closes
the list. Escape and outside interaction close without cancelling a host request.
The host must discard late responses and clear private props on identity changes.
Unmount or key the control by account/session when that identity changes.

`label` must distinguish product navigation from workspace selection and include
OceanWay for product navigation. It is combined with the full current name in the
trigger's accessible label. Native buttons provide Enter, Space and Tab support;
Escape restores focus, and ArrowDown from an open trigger enters the list.
`compact` visually hides the current name while retaining the accessible label.
`defaultOpen` is for previews. Team avatar marks are 30% of the personal avatar,
with a thin surface-colored ring. The default footer avatar is 28px.

Theme via the shared account panel's inherited `--account-surface`, `--account-text`,
`--account-secondary`, `--account-edge`, `--account-accent`,
`--account-card` on the host theme root shared by the trigger and portal. The
`--ow-switcher-*` variables are internal mappings, not a host override interface. Width
follows the host slot; rows use a compact 36px height with native keyboard activation. Product
panels should be mounted in an unclipped toolbar slot. Popovers shift horizontally,
flip above the trigger when needed and scroll within the remaining visual viewport.
Workspace options scroll in their own popup. Pass the same theme root as
`portalContainer` to the account popover and switchers, outside scrolling containers.
Radix handles nested Escape, outside clicks, viewport collision and focus restoration.

The local component lab is `/components/context-switcher`; it uses synthetic
choices and explicit confirmation controls, with no real account or team writes.
