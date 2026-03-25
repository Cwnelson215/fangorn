import type { Account, Transaction, Dashboard, Transfer } from './types';

const BASE = '';

async function fetchJSON<T>(url: string, init?: RequestInit): Promise<T> {
	const res = await fetch(BASE + url, init);
	if (!res.ok) {
		const body = await res.json().catch(() => ({ error: res.statusText }));
		throw new Error(body.error || res.statusText);
	}
	return res.json();
}

export async function createLinkToken(): Promise<string> {
	const data = await fetchJSON<{ link_token: string }>('/api/link-token', { method: 'POST' });
	return data.link_token;
}

export async function exchangeToken(publicToken: string, institutionId: string, institutionName: string): Promise<void> {
	await fetchJSON('/api/exchange-token', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({
			public_token: publicToken,
			institution_id: institutionId,
			institution_name: institutionName
		})
	});
}

export async function getAccounts(): Promise<Account[]> {
	return fetchJSON<Account[]>('/api/accounts');
}

export async function getTransactions(params?: {
	account_id?: number;
	from?: string;
	to?: string;
	category?: string;
	search?: string;
	limit?: number;
	offset?: number;
}): Promise<Transaction[]> {
	const searchParams = new URLSearchParams();
	if (params) {
		for (const [key, value] of Object.entries(params)) {
			if (value !== undefined && value !== null && value !== '') {
				searchParams.set(key, String(value));
			}
		}
	}
	const qs = searchParams.toString();
	return fetchJSON<Transaction[]>(`/api/transactions${qs ? '?' + qs : ''}`);
}

export async function syncAll(): Promise<void> {
	await fetchJSON('/api/sync', { method: 'POST' });
}

export async function getDashboard(from?: string, to?: string): Promise<Dashboard> {
	const params = new URLSearchParams();
	if (from) params.set('from', from);
	if (to) params.set('to', to);
	const qs = params.toString();
	return fetchJSON<Dashboard>(`/api/dashboard${qs ? '?' + qs : ''}`);
}

export async function createTransfer(sourceAccountId: number, destinationAccountId: number, amount: number, description?: string): Promise<Transfer> {
	return fetchJSON<Transfer>('/api/transfers', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({
			source_account_id: sourceAccountId,
			destination_account_id: destinationAccountId,
			amount,
			description: description || ''
		})
	});
}

export async function getTransfers(limit?: number, offset?: number): Promise<Transfer[]> {
	const params = new URLSearchParams();
	if (limit) params.set('limit', String(limit));
	if (offset) params.set('offset', String(offset));
	const qs = params.toString();
	return fetchJSON<Transfer[]>(`/api/transfers${qs ? '?' + qs : ''}`);
}

export async function refreshTransfer(id: number): Promise<Transfer> {
	return fetchJSON<Transfer>(`/api/transfers/${id}/refresh`, { method: 'POST' });
}

export async function cancelTransfer(id: number): Promise<void> {
	await fetchJSON(`/api/transfers/${id}/cancel`, { method: 'POST' });
}
