import { describe, expect, it } from 'vitest';
import { isAppleMobile } from './device';

describe('isAppleMobile', () => {
	it('recognises an iPhone', () => {
		expect(
			isAppleMobile({
				userAgent:
					'Mozilla/5.0 (iPhone; CPU iPhone OS 19_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/19.0 Mobile/15E148 Safari/604.1',
				platform: 'iPhone',
				maxTouchPoints: 5
			})
		).toBe(true);
	});

	it('recognises an iPad that reports itself as a Mac', () => {
		expect(
			isAppleMobile({
				userAgent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/19.0 Safari/605.1.15',
				platform: 'MacIntel',
				maxTouchPoints: 5
			})
		).toBe(true);
	});

	it('leaves out a real Mac, Android and Linux', () => {
		expect(
			isAppleMobile({
				userAgent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/19.0 Safari/605.1.15',
				platform: 'MacIntel',
				maxTouchPoints: 0
			})
		).toBe(false);
		expect(
			isAppleMobile({
				userAgent: 'Mozilla/5.0 (Linux; Android 16; Pixel 9a) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0 Mobile Safari/537.36',
				platform: 'Linux armv8l',
				maxTouchPoints: 5
			})
		).toBe(false);
		expect(isAppleMobile({ userAgent: 'Mozilla/5.0 (X11; Linux x86_64)', platform: 'Linux x86_64', maxTouchPoints: 0 })).toBe(false);
	});

	it('is false without a navigator', () => {
		expect(isAppleMobile(null)).toBe(false);
	});
});
