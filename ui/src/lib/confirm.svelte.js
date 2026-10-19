// confirm({title, message, confirmLabel, destructive}) → Promise<boolean>,
// drawn by <ConfirmHost /> as the admin's action dialog.
class ConfirmState {
  current = $state(null);
}
export const confirmState = new ConfirmState();

export function confirm(options) {
  return new Promise((resolve) => {
    confirmState.current = { confirmLabel: "Confirm", ...options, resolve };
  });
}
