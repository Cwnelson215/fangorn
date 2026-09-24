// The account grouping chosen on this device. Shared state, so the switch on
// the accounts page or dashboard regroups every list and picker at once.
//
// Like remember.ts, storage is a convenience: it can be missing or throw, and
// the default is used then.

import type { AccountGroupBy } from "./grouping";

const KEY = "fangorn.accountGrouping";

function recall(): AccountGroupBy {
  try {
    return localStorage.getItem(KEY) === "institution" ? "institution" : "type";
  } catch {
    return "type";
  }
}

export const grouping = $state<{ by: AccountGroupBy }>({ by: recall() });

export function setGrouping(by: AccountGroupBy) {
  grouping.by = by;
  try {
    localStorage.setItem(KEY, by);
  } catch {
    // Not remembering is fine.
  }
}
