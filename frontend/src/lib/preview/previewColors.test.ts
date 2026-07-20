import { describe, expect, it } from 'vitest';
import { vectorOrientationColor } from './previewColors';

function toHex({ r, g, b }: ReturnType<typeof vectorOrientationColor>) {
	return `#${[r, g, b]
		.map((component) =>
			Math.round(component * 255)
				.toString(16)
				.padStart(2, '0')
		)
		.join('')}`;
}

describe('vectorOrientationColor', () => {
	it('matches the original HSLMap axis colors', () => {
		expect(toHex(vectorOrientationColor(1, 0, 0))).toBe('#ff0000');
		expect(toHex(vectorOrientationColor(0, 1, 0))).toBe('#7fff00');
		expect(toHex(vectorOrientationColor(0, 0, 1))).toBe('#ffffff');
		expect(toHex(vectorOrientationColor(0, 0, -1))).toBe('#000000');
	});

	it('keeps a small out-of-plane tilt as saturated as the original palette', () => {
		const z = 0.05;
		const x = Math.sqrt(1 - z * z);

		expect(toHex(vectorOrientationColor(x, 0, z))).toBe('#ff0c0c');
	});

	it('uses full vector magnitude for saturation, not only the XY projection', () => {
		const z = 0.5;
		const x = Math.sqrt(1 - z * z);

		expect(toHex(vectorOrientationColor(x, 0, z))).toBe('#ff7f7f');
	});
});
