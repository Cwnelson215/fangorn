// Account groups for lists and pickers: by type (Checking, Savings, …) or by
// institution (Gesa, Fidelity, …). Pure so it can be tested; the remembered
// choice lives in grouping.svelte.ts.

import { ACCOUNT_TYPE_LABELS, type Account, type AccountType } from "./types";

export type AccountGroupBy = "type" | "institution";

export interface AccountGroup<A extends Account = Account> {
  key: string;
  label: string;
  items: A[];
  /** Signed sum of balances, so a bank's card nets against its checking. */
  total: number;
}

const TYPE_ORDER = Object.keys(ACCOUNT_TYPE_LABELS) as AccountType[];
const NO_INSTITUTION = "";

export function groupAccounts<A extends Account>(
  accounts: A[],
  by: AccountGroupBy,
): AccountGroup<A>[] {
  const groups = new Map<string, AccountGroup<A>>();
  for (const account of accounts) {
    let key: string;
    let label: string;
    if (by === "type") {
      key = account.type;
      label = ACCOUNT_TYPE_LABELS[account.type];
    } else {
      label = account.institution_name?.trim() ?? "";
      key = label.toLowerCase();
      if (key === NO_INSTITUTION) label = "Other";
    }
    let group = groups.get(key);
    if (!group) {
      group = { key, label, items: [], total: 0 };
      groups.set(key, group);
    }
    group.items.push(account);
    group.total += account.balance;
  }

  const list = [...groups.values()];
  for (const g of list) {
    g.items.sort((a, b) => a.name.localeCompare(b.name));
    g.total = Math.round(g.total * 100) / 100;
  }
  if (by === "type") {
    list.sort(
      (a, b) =>
        TYPE_ORDER.indexOf(a.key as AccountType) -
        TYPE_ORDER.indexOf(b.key as AccountType),
    );
  } else {
    list.sort((a, b) => {
      if (a.key === NO_INSTITUTION) return 1;
      if (b.key === NO_INSTITUTION) return -1;
      return a.label.localeCompare(b.label);
    });
  }
  return list;
}

/** Distinct institution names in use, first spelling wins, for the form's suggestions. */
export function institutionNames(accounts: Account[]): string[] {
  return groupAccounts(accounts, "institution")
    .filter((g) => g.key !== NO_INSTITUTION)
    .map((g) => g.label);
}
