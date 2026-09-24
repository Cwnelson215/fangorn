import { describe, expect, it } from 'vitest';
import { fitWithin, MAX_EDGE } from './image';

describe('fitWithin', () => {
	it('leaves an image that already fits alone', () => {
		expect(fitWithin(1200, 1600, MAX_EDGE)).toEqual({ width: 1200, height: 1600 });
	});

	it('never scales up', () => {
		expect(fitWithin(10, 20, MAX_EDGE)).toEqual({ width: 10, height: 20 });
	});

	it('scales a portrait photo by its height', () => {
		// A 12MP iPhone photo held upright.
		expect(fitWithin(3024, 4032, MAX_EDGE)).toEqual({ width: 1932, height: 2576 });
	});

	it('scales a landscape photo by its width', () => {
		expect(fitWithin(4032, 3024, MAX_EDGE)).toEqual({ width: 2576, height: 1932 });
	});

	it('keeps a long thin receipt at least a pixel wide', () => {
		const out = fitWithin(3, 40000, MAX_EDGE);
		expect(out.height).toBe(MAX_EDGE);
		expect(out.width).toBeGreaterThanOrEqual(1);
	});
});
