// Mirrors internal/models. Amounts are signed relative to the account:
// positive = money in, negative = money out. Liability accounts (credit_card,
// loan) therefore carry negative balances.

export type AccountType =
	| 'checking'
	| 'savings'
	| 'high_yield_savings'
	| 'cash'
	| 'investment'
	| 'retirement'
	| 'credit_card'
	| 'loan';

export type AccountClass = 'asset' | 'liability';

/** How a retirement account is taxed; null on every other type. */
export type TaxTreatment = 'roth' | 'traditional';

export const TAX_TREATMENT_LABELS: Record<TaxTreatment, string> = {
	roth: 'Roth',
	traditional: 'Traditional'
};

/**
 * Whether an account keeps trades and holdings. A retirement account is an
 * investment account with rules on top, so ask this rather than comparing the
 * type to 'investment' (mirrors models.HoldsSecurities).
 */
export function holdsSecurities(type: AccountType): boolean {
	return type === 'investment' || type === 'retirement';
}
export type Kind = 'income' | 'expense' | 'transfer';
/**
 * A transaction can also be the cash side of a trade, or a refund; rules and
 * occurrences can be neither. A refund is money coming back from a category
 * already spent in — positive like income, but it reduces that category's
 * spending rather than adding to what the household earned.
 */
export type TransactionKind = Kind | 'trade' | 'refund';
export type CategoryKind = 'income' | 'expense';

export type Frequency =
	| 'daily'
	| 'weekly'
	| 'biweekly'
	| 'semimonthly'
	| 'monthly'
	| 'quarterly'
	| 'yearly';

export const ACCOUNT_TYPE_LABELS: Record<AccountType, string> = {
	checking: 'Checking',
	savings: 'Savings',
	high_yield_savings: 'High-Yield Savings',
	cash: 'Cash',
	investment: 'Investment',
	retirement: 'Retirement',
	credit_card: 'Credit Card',
	loan: 'Loan'
};

/** What an account is, for badges and headers: "Roth retirement", "Checking". */
export function accountKindLabel(account: Pick<Account, 'type' | 'tax_treatment'>): string {
	if (account.type === 'retirement' && account.tax_treatment) {
		return `${TAX_TREATMENT_LABELS[account.tax_treatment]} retirement`;
	}
	return ACCOUNT_TYPE_LABELS[account.type] ?? account.type;
}

export const FREQUENCY_LABELS: Record<Frequency, string> = {
	daily: 'Daily',
	weekly: 'Weekly',
	biweekly: 'Every 2 weeks',
	semimonthly: 'Twice a month',
	monthly: 'Monthly',
	quarterly: 'Quarterly',
	yearly: 'Yearly'
};

export interface Account {
	id: number;
	household_id: number;
	name: string;
	institution_name: string | null;
	type: AccountType;
	class: AccountClass;
	mask: string | null;
	starting_balance: number;
	starting_balance_date: string;
	currency: string;
	color: string | null;
	notes: string | null;
	tax_treatment: TaxTreatment | null;
	/** A high-yield savings account's current APY in percent (4.35 = 4.35%). */
	apy: number | null;
	/** The money market fund an investment account's cash sits in (SPAXX). */
	cash_fund: string | null;
	archived: boolean;
	/** starting_balance plus every transaction. */
	cash_balance: number;
	/** Market value of an investment account's holdings; 0 for other types. */
	holdings_value: number;
	/** cash_balance + holdings_value — the figure net worth adds up. */
	balance: number;
}

export interface Category {
	id: number;
	name: string;
	kind: CategoryKind;
	color: string | null;
	parent_id: number | null;
	/**
	 * The account this category's spending goes on: receipts filed under it post
	 * there and /add switches to it. The iPhone Shortcut ignores it.
	 */
	default_account_id: number | null;
	archived: boolean;
}

export interface Transaction {
	id: number;
	account_id: number;
	account_name?: string;
	date: string;
	amount: number;
	kind: TransactionKind;
	description: string;
	merchant: string | null;
	category_id: number | null;
	category_name?: string | null;
	notes: string | null;
	transfer_group_id: string | null;
	recurring_rule_id: number | null;
	trade_id: number | null;
	source: 'manual' | 'recurring' | 'receipt' | 'interest';
	created_at: string;
	/** The photographed receipt this was posted from, if any. */
	receipt_id: number | null;
	/** Only present in the per-account register view. */
	running_balance?: number;
}

export type ReceiptStatus = 'pending' | 'processing' | 'needs_review' | 'posted';

export interface ReceiptLineItem {
	description: string;
	quantity: number | null;
	amount: number;
}

/**
 * A photographed receipt and what was read from it. The extracted fields are a
 * record of the reading; once posted, the transaction is what counts.
 */
export interface Receipt {
	id: number;
	status: ReceiptStatus;
	/** Why it is waiting for a person. Empty unless status is needs_review. */
	review_reasons: string[];
	media_type: string;
	byte_size: number;
	merchant: string | null;
	purchased_on: string | null;
	currency: string | null;
	txn_type: 'purchase' | 'return' | null;
	subtotal: number | null;
	tax: number | null;
	tip: number | null;
	total: number | null;
	tender: string | null;
	card_last4: string | null;
	category_suggested: string | null;
	line_items: ReceiptLineItem[] | null;
	model: string | null;
	account_id: number | null;
	category_id: number | null;
	transaction_id: number | null;
	extract_error: string | null;
	created_at: string;
	/** Uploaded by the iPhone Shortcut, so category accounts don't apply. */
	via_shortcut: boolean;
}

export interface ReceiptUpload {
	receipt: Receipt;
	/** This exact photo was uploaded before; `receipt` is that earlier upload. */
	duplicate: boolean;
	/** False when automatic reading is switched off on the server. */
	enabled: boolean;
}

export interface Transfer {
	group_id: string;
	from_account_id: number;
	from_account: string;
	to_account_id: number;
	to_account: string;
	amount: number;
	date: string;
	description: string;
	notes: string | null;
}

export interface RecurringRule {
	id: number;
	household_id: number;
	name: string;
	vendor: string | null;
	kind: Kind;
	account_id: number;
	account_name?: string;
	to_account_id: number | null;
	to_account_name?: string | null;
	category_id: number | null;
	category_name?: string | null;
	amount: number;
	frequency: Frequency;
	interval_count: number;
	day_of_month: number | null;
	second_day_of_month: number | null;
	day_of_week: number | null;
	month_of_year: number | null;
	start_date: string;
	end_date: string | null;
	next_due_date: string | null;
	auto_post: boolean;
	reminder_lead_days: number | null;
	paused: boolean;
	notes: string | null;
}

export interface Occurrence {
	id: number;
	rule_id: number;
	rule_name?: string;
	kind?: Kind;
	amount?: number;
	account_name?: string;
	to_account_name?: string | null;
	due_date: string;
	status: 'scheduled' | 'posted' | 'skipped';
	transaction_id: number | null;
}

/**
 * A monthly amount planned for a category: a spending limit on an expense
 * category, or the income expected on an income category.
 */
export interface Budget {
	id: number;
	category_id: number;
	category_name?: string;
	category_color?: string | null;
	kind: CategoryKind;
	period: 'monthly';
	amount: number;
	effective_from: string;
	/** The month's actual: spend net of refunds, or income received. */
	spent: number;
	/** Recurring expenses (or income) in the month not posted yet. Always 0 for past months. */
	scheduled: number;
}

export interface BudgetMonth {
	/** First of the month, YYYY-MM-DD. */
	month: string;
	budgets: Budget[];
	/** Expense spend in categories with no budget, uncategorized included. */
	unbudgeted_spent: number;
	/** All income in the month. */
	income_received: number;
	/** Income in categories with no expected-income budget, uncategorized included. */
	unplanned_income: number;
	/** Open goals with a monthly amount: planned vs put toward them this month. */
	savings: SavingsLine[];
}

export interface Goal {
	id: number;
	name: string;
	target_amount: number;
	target_date: string | null;
	account_id: number | null;
	account_name?: string | null;
	notes: string | null;
	achieved: boolean;
	/**
	 * Progress toward the target, which is how much to ADD: money added to the
	 * linked account since started_on (transfers in less out, plus income
	 * deposited there, not interest), or contributions logged by hand when
	 * there's no account.
	 */
	saved: number;
	started_on: string;
	/** The goal's line in the monthly budget; null for none. */
	monthly_amount: number | null;
}

/** One goal's share of a month's budget. */
export interface SavingsLine {
	goal_id: number;
	name: string;
	account_id: number | null;
	account_name: string | null;
	monthly_amount: number;
	/** Put toward the goal this month. */
	moved: number;
	saved: number;
	target_amount: number;
}

/** Household-wide choices. */
export interface Settings {
	/** Where income is logged by default, and savings are moved from. */
	income_account_id: number | null;
}

export interface CategorySpend {
	category_id: number | null;
	category_name: string;
	color: string | null;
	amount: number;
}

export interface WeekSpend {
	week: string; // the Monday the week starts on
	amount: number;
}

export interface NetWorthPoint {
	date: string;
	total_assets: number;
	total_liabilities: number;
	net_worth: number;
}

export interface Dashboard {
	from: string;
	to: string;
	income: number;
	expenses: number;
	net: number;
	total_assets: number;
	total_liabilities: number;
	net_worth: number;
	/** The part of net_worth held in retirement accounts. */
	retirement_value: number;
	accounts: Account[];
	categories: CategorySpend[];
	weekly_spending: WeekSpend[];
	net_worth_history: NetWorthPoint[];
	budgets: Budget[];
	goals: Goal[];
	upcoming: Occurrence[];
	/** Null when the household has no investment accounts. Stored prices only. */
	investments: InvestmentsGlance | null;
}

export interface InvestmentsGlance {
	total_value: number;
	day_change: number;
	day_change_pct: number | null;
	as_of: string | null;
}

/** One wedge of a DonutChart. */
export interface Slice {
	label: string;
	value: number;
	color?: string | null;
}

// ---------------------------------------------------------------------------
// investments
// ---------------------------------------------------------------------------

/** buy and sell move the account's cash; reinvest and opening only add shares. */
export type TradeSide = 'buy' | 'sell' | 'reinvest' | 'opening';

export const TRADE_SIDE_LABELS: Record<TradeSide, string> = {
	buy: 'Buy',
	sell: 'Sell',
	reinvest: 'Reinvested dividend',
	opening: 'Already owned'
};

export interface Trade {
	id: number;
	account_id: number;
	symbol: string;
	security_name: string | null;
	side: TradeSide;
	trade_date: string;
	shares: number;
	price: number;
	fees: number;
	/** Dollars paid (buy, incl. fees), received (sell, after fees), or cost basis. */
	amount: number;
	notes: string | null;
	created_at: string;
}

export interface SecurityMatch {
	symbol: string;
	name: string;
	quote_type: string;
	exchange: string;
}

export interface Security {
	symbol: string;
	name: string | null;
	quote_type: string | null;
	currency: string;
	exchange: string | null;
	price: number;
	previous_close: number | null;
	/** Null when the price was taken from a trade and never actually quoted. */
	price_time: string | null;
	fetch_error: string | null;
}

export interface HoldingPosition {
	symbol: string;
	name: string | null;
	quote_type: string | null;
	shares: number;
	avg_cost: number;
	cost_basis: number;
	price: number;
	previous_close: number | null;
	market_value: number;
	day_change: number | null;
	/** Ratios, not percentages: 0.012 is 1.2%. */
	day_change_pct: number | null;
	unrealized_gain: number;
	unrealized_gain_pct: number | null;
	realized_gain: number;
	weight: number;
	price_time: string | null;
	fetch_error: string | null;
}

export interface Holdings {
	account_id: number;
	cash: number;
	holdings_value: number;
	total_value: number;
	cost_basis: number;
	unrealized_gain: number;
	realized_gain: number;
	day_change: number;
	day_change_pct: number | null;
	as_of: string | null;
	seeded: boolean;
	positions: HoldingPosition[];
}

export interface InvestmentAccountValue {
	id: number;
	name: string;
	institution_name: string | null;
	cash: number;
	holdings_value: number;
	total_value: number;
	day_change: number;
	day_change_pct: number | null;
}

/** Every investment account combined; account_id is absent. */
export interface InvestmentsSummary extends Omit<Holdings, 'account_id'> {
	accounts: InvestmentAccountValue[];
}

/** An investment account's (or all of them combined) worth at the end of a day. */
export interface ValuePoint {
	date: string;
	cash: number;
	holdings: number;
	value: number;
}

export interface AccountDetail {
	account: Account;
	transactions: Transaction[];
}

// ---------------------------------------------------------------------------
// write payloads
// ---------------------------------------------------------------------------

export interface AccountInput {
	name: string;
	institution_name: string | null;
	type: AccountType;
	mask: string | null;
	starting_balance: number;
	starting_balance_date: string;
	currency: string;
	color: string | null;
	notes: string | null;
	tax_treatment: TaxTreatment | null;
	/** The opening APY of a new high-yield savings account; create only. */
	apy?: number | null;
	/** Where an investment account's cash sits; null for none. */
	cash_fund: string | null;
}

/** One entry in a high-yield savings account's rate history. */
export interface SavingsRate {
	id: number;
	account_id: number;
	apy: number;
	effective_from: string;
}

export interface SavingsOutlook {
	/** Newest first. */
	rates: SavingsRate[];
	/** The last day of this month, when its interest posts. */
	projected_date: string;
	/** This month's interest if the balance stays where it is. */
	projected_amount: number;
	/** The money market fund an investment account's yield is looked up from. */
	cash_fund: string | null;
	/** When the fund took over; hand-entered rates cover the time before. */
	cash_fund_since: string | null;
	/** The fund's latest published yield (a simple annual rate, percent). */
	fund_yield: number | null;
	fund_yield_as_of: string | null;
}

/** Amount is a positive magnitude; the server applies the sign from `kind`. */
export interface TransactionInput {
	account_id: number;
	date: string;
	amount: number;
	/** A refund must carry the expense category the money is coming back from. */
	kind: 'income' | 'expense' | 'refund';
	description: string;
	merchant: string | null;
	category_id: number | null;
	notes: string | null;
}

/** Omit amount to have the server use shares × price ± fees. */
export interface TradeInput {
	symbol: string;
	side: TradeSide;
	trade_date: string;
	shares: number;
	price: number;
	fees: number;
	amount: number | null;
	notes: string | null;
}

export interface TransferInput {
	from_account_id: number;
	to_account_id: number;
	amount: number;
	date: string;
	description: string;
	notes: string | null;
}

export interface RuleInput {
	name: string;
	vendor: string | null;
	kind: Kind;
	account_id: number;
	to_account_id: number | null;
	category_id: number | null;
	amount: number;
	frequency: Frequency;
	interval_count: number;
	day_of_month: number | null;
	second_day_of_month: number | null;
	day_of_week: number | null;
	month_of_year: number | null;
	start_date: string;
	end_date: string | null;
	auto_post: boolean;
	reminder_lead_days: number | null;
	notes: string | null;
}

export interface CategoryInput {
	name: string;
	kind: CategoryKind;
	color: string | null;
	parent_id: number | null;
	default_account_id: number | null;
}

export interface GoalInput {
	name: string;
	target_amount: number;
	target_date: string | null;
	account_id: number | null;
	notes: string | null;
	monthly_amount: number | null;
}

/** A phone's key for the iPhone Shortcut. The key itself is only returned once. */
export interface DeviceKey {
	id: number;
	name: string;
	account_id: number;
	account_name: string;
	created_at: string;
	last_used_at: string | null;
}

export interface DeviceKeyCreated {
	key: DeviceKey;
	token: string;
}
