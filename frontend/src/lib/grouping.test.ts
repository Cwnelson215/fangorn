import { describe, expect, it } from "vitest";
import { groupAccounts, institutionNames } from "./grouping";
import type { Account, AccountType } from "./types";

let nextId = 1;
function account(
  name: string,
  type: AccountType,
  institution: string | null,
  balance = 0,
): Account {
  const liability = type === "credit_card" || type === "loan";
  return {
    id: nextId++,
    household_id: 1,
    name,
    institution_name: institution,
    type,
    class: liability ? "liability" : "asset",
    mask: null,
    starting_balance: balance,
    starting_balance_date: "2026-01-01",
    currency: "USD",
    color: null,
    notes: null,
    tax_treatment: null,
    apy: null,
    cash_fund: null,
    archived: false,
    cash_balance: balance,
    holdings_value: 0,
    balance,
  };
}

const accounts = [
  account("Visa", "credit_card", "Gesa", -250.1),
  account("Brokerage", "investment", "Fidelity", 1000),
  account("Wallet", "cash", null, 40),
  account("Share Savings", "savings", "gesa ", 500),
  account("Everyday", "checking", "Gesa", 1200.2),
];

describe("groupAccounts", () => {
  it("groups by type in the type-label order, assets before liabilities", () => {
    const groups = groupAccounts(accounts, "type");
    expect(groups.map((g) => g.label)).toEqual([
      "Checking",
      "Savings",
      "Cash",
      "Investment",
      "Credit Card",
    ]);
  });

  it("merges institutions across case and whitespace, keeping the first spelling", () => {
    const groups = groupAccounts(accounts, "institution");
    expect(groups.map((g) => g.label)).toEqual(["Fidelity", "Gesa", "Other"]);
    const gesa = groups[1];
    expect(gesa.items.map((a) => a.name)).toEqual([
      "Everyday",
      "Share Savings",
      "Visa",
    ]);
  });

  it("nets a group total with signed balances", () => {
    const gesa = groupAccounts(accounts, "institution").find(
      (g) => g.key === "gesa",
    )!;
    expect(gesa.total).toBe(1450.1);
  });

  it("puts accounts with no institution last", () => {
    const groups = groupAccounts(accounts, "institution");
    expect(groups.at(-1)?.label).toBe("Other");
    expect(groups.at(-1)?.items.map((a) => a.name)).toEqual(["Wallet"]);
  });

  it("returns nothing for no accounts", () => {
    expect(groupAccounts([], "type")).toEqual([]);
    expect(groupAccounts([], "institution")).toEqual([]);
  });
});

describe("institutionNames", () => {
  it("lists distinct names without the empty group", () => {
    expect(institutionNames(accounts)).toEqual(["Fidelity", "Gesa"]);
  });
});

describe("retirement accounts", () => {
  it("group on their own, right after investment accounts", () => {
    const roth = {
      ...account("Roth IRA", "retirement", "Fidelity", 800),
      tax_treatment: "roth" as const,
    };
    const labels = groupAccounts([...accounts, roth], "type").map(
      (g) => g.label,
    );
    expect(labels).toEqual([
      "Checking",
      "Savings",
      "Cash",
      "Investment",
      "Retirement",
      "Credit Card",
    ]);
  });
});
