import type {
	SavingsOutlook,
	SavingsRate,
	Account,
	AccountDetail,
	AccountInput,
	Budget,
	BudgetMonth,
	Category,
	CategoryInput,
	Dashboard,
	DeviceKey,
	DeviceKeyCreated,
	Goal,
	GoalInput,
	Holdings,
	InvestmentsSummary,
	Occurrence,
	Receipt,
	ReceiptStatus,
	ReceiptUpload,
	RecurringRule,
	RuleInput,
	Security,
	SecurityMatch,
	Trade,
	TradeInput,
	Transaction,
	TransactionInput,
	Transfer,
	TransferInput,
	ValuePoint
} from './types';

// Same-origin: in dev, Vite proxies /api to :3000; in production the Go binary
// serves the built frontend itself. Either way there is no base URL to configure.
const BASE = '';

async function request<T>(url: string, init?: RequestInit): Promise<T> {
	const res = await fetch(BASE + url, init);
	if (!res.ok) {
		const body = await res.json().catch(() => ({ error: res.statusText }));
		throw new Error(body.error || res.statusText);
	}
	if (res.status === 204) return undefined as T;
	return res.json();
}

function send<T>(method: string, url: string, body?: unknown): Promise<T> {
	return request<T>(url, {
		method,
		headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
		body: body === undefined ? undefined : JSON.stringify(body)
	});
}

type QueryValue = string | number | boolean | undefined | null;

/** Builds a query string, dropping empty values so filters can be cleared. */
function qs(params: Record<string, QueryValue> | object): string {
	const search = new URLSearchParams();
	for (const [key, value] of Object.entries(params)) {
		if (value !== undefined && value !== null && value !== '') {
			search.set(key, String(value));
		}
	}
	const s = search.toString();
	return s ? '?' + s : '';
}

// ---------------------------------------------------------------------------
// auth
// ---------------------------------------------------------------------------

export const login = (password: string) => send<{ status: string }>('POST', '/api/login', { password });
export const logout = () => send<{ status: string }>('POST', '/api/logout');
export const authStatus = () =>
	request<{ authenticated: boolean; required: boolean }>('/api/auth/status');

// ---------------------------------------------------------------------------
// dashboard
// ---------------------------------------------------------------------------

export const getDashboard = (from?: string, to?: string) =>
	request<Dashboard>(`/api/dashboard${qs({ from, to })}`);

// ---------------------------------------------------------------------------
// accounts
// ---------------------------------------------------------------------------

export const getAccounts = (includeArchived = false) =>
	request<Account[]>(`/api/accounts${qs({ include_archived: includeArchived || undefined })}`);

export const getAccount = (id: number, limit?: number) =>
	request<AccountDetail>(`/api/accounts/${id}${qs({ limit })}`);

export const createAccount = (input: AccountInput) => send<Account>('POST', '/api/accounts', input);
export const updateAccount = (id: number, input: AccountInput) =>
	send<Account>('PATCH', `/api/accounts/${id}`, input);
export const deleteAccount = (id: number) => send<void>('DELETE', `/api/accounts/${id}`);
export const archiveAccount = (id: number) => send<Account>('POST', `/api/accounts/${id}/archive`);
export const unarchiveAccount = (id: number) => send<Account>('POST', `/api/accounts/${id}/unarchive`);

export const getSavingsOutlook = (accountId: number) =>
	request<SavingsOutlook>(`/api/accounts/${accountId}/savings`);
export const addSavingsRate = (accountId: number, input: { apy: number; effective_from: string }) =>
	send<SavingsRate>('POST', `/api/accounts/${accountId}/rates`, input);
export const deleteSavingsRate = (accountId: number, rateId: number) =>
	send<void>('DELETE', `/api/accounts/${accountId}/rates/${rateId}`);

// ---------------------------------------------------------------------------
// investments
// ---------------------------------------------------------------------------

export const searchSecurities = (q: string, signal?: AbortSignal) =>
	request<SecurityMatch[]>(`/api/securities/search${qs({ q })}`, { signal });
export const getSecurity = (symbol: string) =>
	request<Security>(`/api/securities/${encodeURIComponent(symbol)}`);

export const getHoldings = (accountId: number) =>
	request<Holdings>(`/api/accounts/${accountId}/holdings`);
export const getTrades = (accountId: number) => request<Trade[]>(`/api/accounts/${accountId}/trades`);
export const createTrade = (accountId: number, input: TradeInput) =>
	send<Trade>('POST', `/api/accounts/${accountId}/trades`, input);
export const updateTrade = (id: number, input: TradeInput) =>
	send<Trade>('PATCH', `/api/trades/${id}`, input);
export const deleteTrade = (id: number) => send<void>('DELETE', `/api/trades/${id}`);
export const getInvestments = () => request<InvestmentsSummary>('/api/investments');
/** days <= 0 means all history. */
export const getInvestmentsHistory = (days: number) =>
	request<ValuePoint[]>(`/api/investments/value-history${qs({ days })}`);
export const getAccountValueHistory = (accountId: number, days: number) =>
	request<ValuePoint[]>(`/api/accounts/${accountId}/value-history${qs({ days })}`);

// ---------------------------------------------------------------------------
// categories
// ---------------------------------------------------------------------------

export const getCategories = (includeArchived = false) =>
	request<Category[]>(`/api/categories${qs({ include_archived: includeArchived || undefined })}`);

export const createCategory = (input: CategoryInput) =>
	send<Category>('POST', '/api/categories', input);
export const updateCategory = (id: number, input: CategoryInput) =>
	send<Category>('PATCH', `/api/categories/${id}`, input);
export const deleteCategory = (id: number) => send<void>('DELETE', `/api/categories/${id}`);
export const unarchiveCategory = (id: number) =>
	send<Category>('POST', `/api/categories/${id}/unarchive`);

// ---------------------------------------------------------------------------
// transactions
// ---------------------------------------------------------------------------

export interface TransactionQuery {
	account_id?: number;
	category_id?: number;
	kind?: string;
	from?: string;
	to?: string;
	search?: string;
	limit?: number;
}

export const getTransactions = (params: TransactionQuery = {}) =>
	request<Transaction[]>(`/api/transactions${qs(params)}`);

export const createTransaction = (input: TransactionInput) =>
	send<Transaction>('POST', '/api/transactions', input);
export const updateTransaction = (id: number, input: TransactionInput) =>
	send<Transaction>('PATCH', `/api/transactions/${id}`, input);
export const deleteTransaction = (id: number) => send<void>('DELETE', `/api/transactions/${id}`);

// ---------------------------------------------------------------------------
// receipts
// ---------------------------------------------------------------------------

/**
 * Uploads one receipt photo. The server tries to read it before answering, so
 * this can take several seconds; if it is still being read, the returned
 * receipt's status is pending or processing and the caller polls getReceipt.
 */
export function uploadReceipt(image: Blob): Promise<ReceiptUpload> {
	const form = new FormData();
	form.append('image', image, 'receipt.jpg');
	// No Content-Type header: the browser sets the multipart boundary itself.
	return request<ReceiptUpload>('/api/receipts', { method: 'POST', body: form });
}

export const getReceipts = (status?: ReceiptStatus) =>
	request<Receipt[]>(`/api/receipts${qs({ status })}`);
export const getReceipt = (id: number) => request<Receipt>(`/api/receipts/${id}`);
export const receiptImageUrl = (id: number) => `${BASE}/api/receipts/${id}/image`;
/** Posts a receipt held for review, as the person confirmed it. */
export const postReceipt = (id: number, input: TransactionInput) =>
	send<Transaction>('POST', `/api/receipts/${id}/post`, input);
export const retryReceipt = (id: number) => send<Receipt>('POST', `/api/receipts/${id}/retry`);
export const deleteReceipt = (id: number) => send<void>('DELETE', `/api/receipts/${id}`);

// ---------------------------------------------------------------------------
// transfers
// ---------------------------------------------------------------------------

export const getTransfers = (limit?: number) =>
	request<Transfer[]>(`/api/transfers${qs({ limit })}`);

export const createTransfer = (input: TransferInput) =>
	send<Transfer>('POST', '/api/transfers', input);
export const updateTransfer = (groupId: string, input: TransferInput) =>
	send<Transfer>('PATCH', `/api/transfers/${groupId}`, input);
export const deleteTransfer = (groupId: string) => send<void>('DELETE', `/api/transfers/${groupId}`);

// ---------------------------------------------------------------------------
// recurring
// ---------------------------------------------------------------------------

export const getRules = () => request<RecurringRule[]>('/api/recurring');
export const getUpcoming = (days?: number) =>
	request<Occurrence[]>(`/api/recurring/upcoming${qs({ days })}`);

export const createRule = (input: RuleInput) => send<RecurringRule>('POST', '/api/recurring', input);
export const updateRule = (id: number, input: RuleInput) =>
	send<RecurringRule>('PATCH', `/api/recurring/${id}`, input);
export const deleteRule = (id: number) => send<void>('DELETE', `/api/recurring/${id}`);
export const pauseRule = (id: number) => send<RecurringRule>('POST', `/api/recurring/${id}/pause`);
export const resumeRule = (id: number) => send<RecurringRule>('POST', `/api/recurring/${id}/resume`);
export const skipRule = (id: number) => send<RecurringRule>('POST', `/api/recurring/${id}/skip`);
export const postRuleNow = (id: number) => send<Occurrence>('POST', `/api/recurring/${id}/post-now`);

// ---------------------------------------------------------------------------
// budgets and goals
// ---------------------------------------------------------------------------

export const getBudgets = (month?: string) =>
	request<BudgetMonth>(`/api/budgets${qs({ month })}`);
export const setBudget = (categoryId: number, amount: number, effectiveFrom: string) =>
	send<Budget>('POST', '/api/budgets', {
		category_id: categoryId,
		amount,
		effective_from: effectiveFrom
	});
/** Stops a budget from `month` onward; earlier months keep it. */
export const stopBudget = (id: number, month: string) =>
	send<void>('DELETE', `/api/budgets/${id}${qs({ month })}`);

export const getGoals = () => request<Goal[]>('/api/goals');
export const createGoal = (input: GoalInput) => send<Goal>('POST', '/api/goals', input);
export const updateGoal = (id: number, input: GoalInput) =>
	send<Goal>('PATCH', `/api/goals/${id}`, input);
export const deleteGoal = (id: number) => send<void>('DELETE', `/api/goals/${id}`);
export const contributeToGoal = (id: number, amount: number, date: string, note?: string) =>
	send<Goal>('POST', `/api/goals/${id}/contribute`, { amount, date, note: note || null });
export const achieveGoal = (id: number) => send<Goal>('POST', `/api/goals/${id}/achieve`);
export const reopenGoal = (id: number) => send<Goal>('POST', `/api/goals/${id}/reopen`);

// ---------------------------------------------------------------------------
// iPhone Shortcut keys
// ---------------------------------------------------------------------------

export const getDeviceKeys = () => request<DeviceKey[]>('/api/device-keys');
export const createDeviceKey = (name: string, account_id: number) =>
	send<DeviceKeyCreated>('POST', '/api/device-keys', { name, account_id });
export const deleteDeviceKey = (id: number) => send<void>('DELETE', `/api/device-keys/${id}`);

/**
 * Calls the Shortcut's own endpoint with a key, the way the phone will, so a
 * new key can be checked before it goes into the Shortcut.
 */
export async function testDeviceKey(token: string): Promise<string[]> {
	const res = await fetch(BASE + '/api/shortcut/categories', {
		headers: { Authorization: `Bearer ${token}` }
	});
	if (!res.ok) throw new Error((await res.text()).trim() || res.statusText);
	return res.json();
}
