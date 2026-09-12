// Mirrors internal/models. Amounts are signed relative to the account:
// positive = money in, negative = money out. Liability accounts (credit_card,
// loan) therefore carry negative balances.

export type AccountType =
	| 'checking'
	| 'savings'
	| 'cash'
	| 'investment'
	| 'credit_card'
	| 'loan';

export type AccountClass = 'asset' | 'liability';
export type Kind = 'income' | 'expense' | 'transfer';
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
	cash: 'Cash',
	investment: 'Investment',
	credit_card: 'Credit Card',
	loan: 'Loan'
};

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
	archived: boolean;
	balance: number;
}

export interface Category {
	id: number;
	name: string;
	kind: CategoryKind;
	color: string | null;
	parent_id: number | null;
	archived: boolean;
}

export interface Transaction {
	id: number;
	account_id: number;
	account_name?: string;
	date: string;
	amount: number;
	kind: Kind;
	description: string;
	merchant: string | null;
	category_id: number | null;
	category_name?: string | null;
	notes: string | null;
	transfer_group_id: string | null;
	recurring_rule_id: number | null;
	source: 'manual' | 'recurring';
	created_at: string;
	/** Only present in the per-account register view. */
	running_balance?: number;
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

export interface Budget {
	id: number;
	category_id: number;
	category_name?: string;
	category_color?: string | null;
	period: 'monthly';
	amount: number;
	effective_from: string;
	spent: number;
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
	saved: number;
}

export interface CategorySpend {
	category_id: number | null;
	category_name: string;
	color: string | null;
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
	accounts: Account[];
	categories: CategorySpend[];
	net_worth_history: NetWorthPoint[];
	budgets: Budget[];
	goals: Goal[];
	upcoming: Occurrence[];
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
}

/** Amount is a positive magnitude; the server applies the sign from `kind`. */
export interface TransactionInput {
	account_id: number;
	date: string;
	amount: number;
	kind: 'income' | 'expense';
	description: string;
	merchant: string | null;
	category_id: number | null;
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
}

export interface GoalInput {
	name: string;
	target_amount: number;
	target_date: string | null;
	account_id: number | null;
	notes: string | null;
}
