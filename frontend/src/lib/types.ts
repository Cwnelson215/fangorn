export interface Account {
	id: number;
	external_account_id: string | null;
	name: string;
	official_name: string | null;
	type: string;
	subtype: string | null;
	mask: string | null;
	current_balance: number | null;
	available_balance: number | null;
	iso_currency_code: string;
	institution_name: string | null;
	source: string;
}

export interface Transaction {
	id: number;
	external_id: string | null;
	account_id: number;
	amount: number;
	iso_currency_code: string;
	date: string;
	name: string;
	merchant_name: string | null;
	category: string | null;
	pending: boolean;
	account_name: string;
	source: string;
}

export interface ImportResult {
	imported: number;
	skipped: number;
	account_id: number;
}

export interface CategoryBreakdown {
	category: string;
	amount: number;
}

export interface NetWorthPoint {
	date: string;
	net_worth: number;
}

export interface Transfer {
	id: number;
	source_account_id: number;
	destination_account_id: number;
	amount: number;
	description: string | null;
	status: string;
	failure_reason: string | null;
	created_at: string;
	updated_at: string;
	source_account_name: string;
	destination_account_name: string;
}

export interface Dashboard {
	from: string;
	to: string;
	income: number;
	expenses: number;
	net: number;
	categories: CategoryBreakdown[];
	net_worth: number | null;
	total_assets: number;
	total_liabilities: number;
	net_worth_history: NetWorthPoint[];
}
