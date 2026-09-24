/// <reference types="@sveltejs/kit" />
/// <reference no-default-lib="true"/>
/// <reference lib="esnext" />
/// <reference lib="webworker" />

// Makes the installed app open like a native one: the code and icons come from
// the phone, so launching doesn't wait on a slow connection to fetch them.
//
// Financial data is never cached. /api/ always goes to the network, and so do
// page loads while the network answers — the cached page is only a fallback,
// so a deploy or a sign-out is never hidden behind a stale copy.

import { build, files, version } from '$service-worker';

const sw = self as unknown as ServiceWorkerGlobalScope;
const CACHE = `fangorn-${version}`;
const SHELL = '/';

// Only files the auth middleware lets through without a session: anything else
// would be cached as the /login redirect when installed from the login page.
const PUBLIC = files.filter((f) => f === '/manifest.webmanifest' || f.startsWith('/icons/'));
const ASSETS = new Set([...build, ...PUBLIC]);

sw.addEventListener('install', (event) => {
	event.waitUntil(
		caches
			.open(CACHE)
			.then((cache) => cache.addAll([...ASSETS]))
			.then(() => sw.skipWaiting())
	);
});

sw.addEventListener('activate', (event) => {
	event.waitUntil(
		caches
			.keys()
			.then((keys) => Promise.all(keys.filter((k) => k !== CACHE).map((k) => caches.delete(k))))
			.then(() => sw.clients.claim())
	);
});

sw.addEventListener('fetch', (event) => {
	const request = event.request;
	if (request.method !== 'GET') return;
	const url = new URL(request.url);
	if (url.origin !== sw.location.origin) return;

	// Hashed build output and icons never change under the same name.
	if (ASSETS.has(url.pathname)) {
		event.respondWith(caches.match(url.pathname).then((hit) => hit ?? fetch(request)));
		return;
	}

	// Every route is the same SPA page. Take the network's copy and keep it as
	// the offline fallback — but only a real page, never a redirect to /login.
	if (request.mode === 'navigate') {
		event.respondWith(
			fetch(request)
				.then((response) => {
					if (response.ok && !response.redirected) {
						const copy = response.clone();
						caches.open(CACHE).then((cache) => cache.put(SHELL, copy));
					}
					return response;
				})
				.catch(async () => (await caches.match(SHELL)) ?? Response.error())
		);
	}
	// Everything else, /api/ included, is left to the browser.
});
