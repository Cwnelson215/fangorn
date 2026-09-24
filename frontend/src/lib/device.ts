// Only iPhones and iPads run Shortcuts, so the Shortcut setup is offered only
// there. iPadOS Safari reports itself as a Mac by default; a "Mac" with a touch
// screen is an iPad. A real Mac could run Shortcuts too, but on a laptop the
// web app is already open, so it isn't offered there.

type NavigatorLike = { userAgent: string; platform?: string; maxTouchPoints?: number };

export function isAppleMobile(nav: NavigatorLike | null = globalThis.navigator ?? null): boolean {
	if (!nav) return false;
	if (/iPhone|iPad|iPod/.test(nav.userAgent)) return true;
	return nav.platform === 'MacIntel' && (nav.maxTouchPoints ?? 0) > 1;
}
