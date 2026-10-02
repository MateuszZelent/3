import { use, init, registerTheme, type ECharts } from 'echarts/core';
import { HeatmapChart, LineChart, ScatterChart } from 'echarts/charts';
import {
	AxisPointerComponent,
	DataZoomInsideComponent,
	DataZoomSliderComponent,
	GridComponent,
	ToolboxComponent,
	TooltipComponent,
	VisualMapComponent
} from 'echarts/components';
import { CanvasRenderer } from 'echarts/renderers';

use([
	LineChart,
	ScatterChart,
	HeatmapChart,
	GridComponent,
	TooltipComponent,
	ToolboxComponent,
	VisualMapComponent,
	AxisPointerComponent,
	DataZoomInsideComponent,
	DataZoomSliderComponent,
	CanvasRenderer
]);

export { init as initECharts, type ECharts };

// Canvas libraries cannot consume CSS variables directly. Resolve the same
// tokens as the shared Svelte controls when building their options.
function uiColor(token: string, fallback: string): string {
	if (typeof document === 'undefined') return fallback;
	return getComputedStyle(document.documentElement).getPropertyValue(token).trim() || fallback;
}
export const THEME = {
	get bg() { return uiColor('--surface-1', '#0f1728'); },
	get surface1() { return uiColor('--surface-1', '#0f1728'); },
	get surface2() { return uiColor('--surface-2', '#141f33'); },
	get surface3() { return uiColor('--surface-3', '#1b2840'); },
	get border() { return uiColor('--border', '#273753'); },
	get borderInteractive() { return uiColor('--border-interactive', '#4d739e'); },
	get text1() { return uiColor('--text-1', '#edf3fb'); },
	get text2() { return uiColor('--text-2', '#a7bad3'); },
	get text3() { return uiColor('--text-3', '#6b7f9f'); },
	get accent() { return uiColor('--accent', '#57c8b6'); },
	get accentHover() { return uiColor('--accent-hover', '#2fa596'); },
	get info() { return uiColor('--info', '#6ba7ff'); },
	get tooltipBg() { return this.surface1; },
	get tooltipBorder() { return this.borderInteractive; },
	get tooltipText() { return this.text1; },
	get toolboxIcon() { return this.text2; },
	get brushBg() { return this.surface3; },
	get brushBorder() { return this.accent; }
};

export const ECHARTS_THEME_NAME = 'amumax-lab';

let registered = false;

export function ensureAmumaxEChartsTheme() {
	if (registered) {
		return;
	}

	registerTheme(ECHARTS_THEME_NAME, {
		color: [THEME.info, THEME.accent, '#7dd3fc', '#34d399'],
		backgroundColor: 'transparent',
		textStyle: {
			color: THEME.text2,
			fontFamily: 'IBM Plex Sans, sans-serif'
		}
	});

	registered = true;
}
