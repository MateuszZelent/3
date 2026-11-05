import Prism from 'prismjs';
import 'prismjs/components/prism-go';
import type { Token, TokenStream } from 'prismjs';

export type ConsoleTokenNode =
	| { kind: 'text'; text: string }
	| { kind: 'token'; className: string; children: ConsoleTokenNode[] };

export function tokenizeConsoleText(source: string): ConsoleTokenNode[] {
	const grammar = Prism.languages['go'];
	if (!grammar) return [{ kind: 'text', text: source }];
	return convertStream(Prism.tokenize(source, grammar));
}

function convertStream(stream: TokenStream): ConsoleTokenNode[] {
	if (typeof stream === 'string') {
		return stream ? [{ kind: 'text', text: stream }] : [];
	}
	if (Array.isArray(stream)) return stream.flatMap(convertStream);
	return [convertToken(stream)];
}

function convertToken(token: Token): ConsoleTokenNode {
	const aliases = Array.isArray(token.alias) ? token.alias : token.alias ? [token.alias] : [];
	return {
		kind: 'token',
		className: ['token', token.type, ...aliases].filter(Boolean).join(' '),
		children: convertStream(token.content)
	};
}
