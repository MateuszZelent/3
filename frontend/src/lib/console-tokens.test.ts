import { describe, expect, it } from 'vitest';
import { tokenizeConsoleText, type ConsoleTokenNode } from './console-tokens';

function textContent(nodes: ConsoleTokenNode[]): string {
	return nodes
		.map((node) => (node.kind === 'text' ? node.text : textContent(node.children)))
		.join('');
}

function classes(nodes: ConsoleTokenNode[]): string[] {
	return nodes.flatMap((node) =>
		node.kind === 'text' ? [] : [node.className, ...classes(node.children)]
	);
}

describe('console syntax tokens', () => {
	it('preserves source text and emits Prism classes without building HTML strings', () => {
		const source = 'func main() { // comment\n println("<img src=x onerror=alert(1)>")\n}\n';
		const tokens = tokenizeConsoleText(source);

		expect(textContent(tokens)).toBe(source);
		expect(classes(tokens)).toContain('token keyword');
		expect(classes(tokens)).toContain('token comment');
		expect(classes(tokens)).toContain('token string');
	});
});
