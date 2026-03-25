export interface Account {
	id: number;
	plaid_account_id: string;
	name: string;
	official_name: string | null;
	type: string;
	subtype: string | null;
	mask: string | null;
	current_balance: number | null;
	available_balance: number | null;
	iso_currency_code: string;
	institution_name: string | null;
}

export interface Transaction {
	id: number;
	plaid_transaction_id: string;
	account_id: number;
	amount: number;
	iso_currency_code: string;
	date: string;
	name: string;
	merchant_name: string | null;
	category: string | null;
	plaid_category: string | null;
	pending: boolean;
	account_name: string;
}

export interface CategoryBreakdown {
	category: string;
	amount: number;
}

export interface NetWorthPoint {
	date: string;
	net_worth: number;
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
