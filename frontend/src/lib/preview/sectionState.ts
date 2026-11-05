import { writable, get } from 'svelte/store';

// Open/closed state of the collapsible preview sections. It lives outside the
// components so it survives 2D/3D switches and viewport changes. The viewport
// width only picks the initial default.
export type PreviewSection = 'volume' | 'render' | 'region' | 'floating';

const KEY = 'preview_sections';
const narrow = () => typeof window !== 'undefined' && window.innerWidth <= 650;

function initial(): Record<PreviewSection, boolean> {
	const defaults = { volume: !narrow(), render: false, region: false, floating: !narrow() };
	if (typeof window === 'undefined') return defaults;
	try {
		const raw = JSON.parse(window.localStorage.getItem(KEY) || '{}');
		for (const key of Object.keys(defaults) as PreviewSection[])
			if (typeof raw[key] === 'boolean') defaults[key] = raw[key];
	} catch {
		// Ignore unreadable stored state.
	}
	return defaults;
}

export const sectionState = writable<Record<PreviewSection, boolean>>(initial());

export function setSectionOpen(section: PreviewSection, open: boolean) {
	if (get(sectionState)[section] === open) return false;
	sectionState.update((state) => ({ ...state, [section]: open }));
	try {
		window.localStorage.setItem(KEY, JSON.stringify(get(sectionState)));
	} catch {
		// Persistence is optional.
	}
	return true;
}

// Opening a section inside the scrollable controls area must reveal its content.
export function toggleSection(section: PreviewSection, details: HTMLDetailsElement) {
	// Ignore the toggle event fired for the initial render (no state change).
	if (setSectionOpen(section, details.open) && details.open)
		details.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
}
