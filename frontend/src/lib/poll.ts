// Periodic refresh for pages showing live data (investment prices).
//
// The rest of the app loads once on mount, so this is kept deliberately small:
// it skips a run while the tab is hidden or the previous run is still going, and
// catches up straight away when the tab becomes visible again if a run was due.

/** Calls fn every `ms` while the page is visible. Returns a stop function. */
export function startPolling(fn: () => Promise<void>, ms: number): () => void {
	let running = false;
	let lastRun = Date.now();

	async function tick() {
		if (running || document.hidden) return;
		running = true;
		lastRun = Date.now();
		try {
			await fn();
		} finally {
			running = false;
		}
	}

	function onVisibility() {
		if (!document.hidden && Date.now() - lastRun >= ms) tick();
	}

	const timer = setInterval(tick, ms);
	document.addEventListener('visibilitychange', onVisibility);
	return () => {
		clearInterval(timer);
		document.removeEventListener('visibilitychange', onVisibility);
	};
}
