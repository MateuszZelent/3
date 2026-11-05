import { describe, expect, it } from 'vitest';
import { readPreviewStorage, writePreviewStorage } from './safePreviewStorage';

describe('preview local storage', () => {
	it('reads and writes when storage is available', () => {
		const values = new Map<string, string>();
		const storage = {
			getItem: (key: string) => values.get(key) ?? null,
			setItem: (key: string, value: string) => values.set(key, value)
		};

		expect(writePreviewStorage('quality', 'low', () => storage)).toBe(true);
		expect(readPreviewStorage('quality', () => storage)).toBe('low');
	});

	it('treats blocked storage getters and operations as unavailable', () => {
		expect(
			readPreviewStorage('quality', () => {
				throw new DOMException('blocked', 'SecurityError');
			})
		).toBeNull();
		expect(
			readPreviewStorage('quality', () => ({
				getItem: () => {
					throw new DOMException('blocked', 'SecurityError');
				},
				setItem: () => undefined
			}))
		).toBeNull();
		expect(
			writePreviewStorage('quality', 'low', () => ({
				getItem: () => null,
				setItem: () => {
					throw new DOMException('blocked', 'SecurityError');
				}
			}))
		).toBe(false);
		expect(writePreviewStorage('quality', 'low', () => null)).toBe(false);
	});
});
