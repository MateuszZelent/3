export const MAX_VISIBLE_CONSOLE_CHARACTERS = 32 * 1024;
export const MAX_VISIBLE_CONSOLE_LINES = 500;
const BACKEND_TRUNCATION_MARKER = '// [console history truncated]';

export interface VisibleConsoleFragment {
	text: string;
	omitted: boolean;
}

export function visibleConsoleFragment(
	history: string,
	limits: { maxCharacters?: number; maxLines?: number } = {}
): VisibleConsoleFragment {
	const maxCharacters = Math.max(
		1,
		Math.floor(limits.maxCharacters ?? MAX_VISIBLE_CONSOLE_CHARACTERS)
	);
	const maxLines = Math.max(1, Math.floor(limits.maxLines ?? MAX_VISIBLE_CONSOLE_LINES));
	const end = history.length;
	const characterStart = Math.max(0, end - maxCharacters);
	let lineStart = characterStart;
	let lineCount = 0;
	let index = end - 1;

	// A trailing newline terminates the last content line; it is not another line.
	if (index >= characterStart && history.charCodeAt(index) === 10) index--;
	for (; index >= characterStart; index--) {
		if (history.charCodeAt(index) === 10 && ++lineCount >= maxLines) {
			lineStart = index + 1;
			break;
		}
	}

	let start = Math.max(characterStart, lineStart);
	if (
		characterStart > 0 &&
		start === characterStart &&
		history.charCodeAt(characterStart - 1) !== 10
	) {
		const boundary = history.indexOf('\n', characterStart);
		if (boundary >= 0) start = boundary + 1;
	}
	if (start > 0 && start < end) {
		const code = history.charCodeAt(start);
		if (code >= 0xdc00 && code <= 0xdfff) start++;
	}
	return {
		text: history.slice(start),
		omitted: start > 0 || history.startsWith(BACKEND_TRUNCATION_MARKER)
	};
}
