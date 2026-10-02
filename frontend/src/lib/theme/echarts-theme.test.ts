import { afterEach, expect, it, vi } from 'vitest';
import { THEME } from './echarts-theme';

afterEach(() => vi.unstubAllGlobals());
it('uses the application tokens for canvas and chart colors', () => {
	vi.stubGlobal('document', { documentElement: {} });
	vi.stubGlobal('getComputedStyle', () => ({
		getPropertyValue: (token: string) => ({
			'--surface-1': '#f0f1f2', '--text-2': '#223344', '--accent': '#123456'
		}[token] || '')
	}));
	expect(THEME.bg).toBe('#f0f1f2');
	expect(THEME.tooltipBg).toBe(THEME.surface1);
	expect(THEME.text2).toBe('#223344');
	expect(THEME.brushBorder).toBe('#123456');
});
it('has matching default colors before DOM initialization', () => {
	vi.stubGlobal('document', undefined);
	expect(THEME.surface1).toBe('#0f1728');
	expect(THEME.text1).toBe('#edf3fb');
});
