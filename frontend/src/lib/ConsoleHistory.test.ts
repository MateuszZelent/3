import { describe, expect, it } from 'vitest';
import { visibleConsoleFragment } from './console-history';

describe('visible console fragment', () => {
	it('keeps only a bounded tail and reports omitted output', () => {
		const history = 'old line\nmiddle line\nrecent one\nrecent two\n';

		const fragment = visibleConsoleFragment(history, { maxCharacters: 32, maxLines: 2 });

		expect(fragment.text).toBe('recent one\nrecent two\n');
		expect(fragment.text.length).toBeLessThanOrEqual(32);
		expect(fragment.omitted).toBe(true);
	});

	it('bounds a single oversized line without scanning or rendering earlier text', () => {
		const fragment = visibleConsoleFragment(`${'x'.repeat(128)}tail`, {
			maxCharacters: 8,
			maxLines: 3
		});

		expect(fragment.text).toBe('xxxxtail');
		expect(fragment.text.length).toBe(8);
		expect(fragment.omitted).toBe(true);
	});

	it('preserves complete recent lines at the character boundary', () => {
		const fragment = visibleConsoleFragment('old\nfirst\nsecond\nthird\n', {
			maxCharacters: 19,
			maxLines: 10
		});

		expect(fragment.text).toBe('first\nsecond\nthird\n');
		expect(fragment.omitted).toBe(true);
	});

	it('reports backend-truncated history even when its marker fits in the visible tail', () => {
		const fragment = visibleConsoleFragment('// [console history truncated]\nrecent output');

		expect(fragment.omitted).toBe(true);
	});

	it('keeps at most 500 recent lines by default', () => {
		const source = Array.from({ length: 501 }, (_, index) => `line ${index}`).join('\n');
		const fragment = visibleConsoleFragment(source);

		expect(fragment.text.split('\n')).toHaveLength(500);
		expect(fragment.text.startsWith('line 1\n')).toBe(true);
		expect(fragment.text.endsWith('line 500')).toBe(true);
	});
});
