/**
 * Fold opened while a tool call awaits the user, folded again once done; a fold the user opened stays open
 * @param {() => boolean} waiting - awaits the user's approval
 * @param {() => boolean} done - calls answered
 * @returns {{ open: boolean }} - to bind to the details' open
 */
export function approvalFold(waiting: () => boolean, done: () => boolean): { open: boolean } {
  const fold = $state({ open: false });
  let openedForApproval = false;
  $effect(() => {
    if (waiting()) {
      fold.open = true;
      openedForApproval = true;
    } else if (openedForApproval && done()) {
      fold.open = false;
      openedForApproval = false;
    }
  });
  return fold;
}
