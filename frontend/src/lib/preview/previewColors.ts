export interface SRGBColor {
	r: number;
	g: number;
	b: number;
}

function modulo(value: number, divisor: number) {
	return ((value % divisor) + divisor) % divisor;
}

function toGoColorComponent(value: number) {
	return Math.floor(255 * value) / 255;
}

/**
 * Exact sRGB port of draw.HSLMap.  The values are quantized like Go's
 * uint8(colorComponent * 255) conversion so the WebGL preview and exported
 * vector image use the same palette.
 */
export function vectorOrientationColor(x: number, y: number, z: number): SRGBColor {
	const saturation = Math.min(1, Math.hypot(x, y, z));
	const lightness = Math.min(1, 0.5 * z + 0.5);
	const hue = modulo(Math.atan2(y, x) / (Math.PI / 3), 6);
	const chroma = lightness <= 0.5 ? 2 * lightness * saturation : (2 - 2 * lightness) * saturation;
	const secondary = chroma * (1 - Math.abs(modulo(hue, 2) - 1));

	let red = 0;
	let green = 0;
	let blue = 0;

	if (hue < 1) {
		[red, green] = [chroma, secondary];
	} else if (hue < 2) {
		[red, green] = [secondary, chroma];
	} else if (hue < 3) {
		[green, blue] = [chroma, secondary];
	} else if (hue < 4) {
		[green, blue] = [secondary, chroma];
	} else if (hue < 5) {
		[red, blue] = [secondary, chroma];
	} else {
		[red, blue] = [chroma, secondary];
	}

	const match = lightness - 0.5 * chroma;
	return {
		r: toGoColorComponent(red + match),
		g: toGoColorComponent(green + match),
		b: toGoColorComponent(blue + match)
	};
}
