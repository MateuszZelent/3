import assert from 'node:assert/strict';
import { createRequire } from 'node:module';
import test from 'node:test';

const require = createRequire(import.meta.url);
const merge = require('deepmerge');
test('Svelte plugin resolves the same hardened merge implementation', () => {
	const pluginRequire = createRequire(
		new URL('../node_modules/@sveltejs/vite-plugin-svelte/package.json', import.meta.url)
	);
	assert.equal(pluginRequire('deepmerge'), merge);
});

const payload = () => JSON.parse('{"__proto__":{"isAdmin":true}}');

for (const [name, target, source] of [
	['null-prototype target', Object.create(null), payload()],
	['own prototype key on target', payload(), {}],
	['own prototype key on both sides', payload(), payload()],
	['nested null placeholder', { session: null }, { session: payload() }],
	['nested undefined placeholder', { session: undefined }, { session: payload() }]
]) {
	test(`merge prevents prototype poisoning with ${name}`, () => {
		const result = merge(target, source);
		const object = name.startsWith('nested') ? result.session : result;
		assert.equal(Object.getPrototypeOf(object), Object.prototype);
		assert.equal(object.isAdmin, undefined);
		assert.equal(Object.hasOwn(object, '__proto__'), false);
		assert.equal(Reflect.get({}, 'isAdmin'), undefined);
	});
}

test('merge drops constructor and prototype keys from both inputs', () => {
	const dangerous = JSON.parse(
		'{"constructor":{"prototype":{"isAdmin":true}},"prototype":{"isAdmin":true}}'
	);
	assert.deepEqual(merge(dangerous, dangerous), {});
});

test('merge fails closed when property lookup throws', () => {
	const target = new Proxy(
		{},
		{
			has() {
				throw new Error('denied');
			}
		}
	);
	assert.deepEqual(merge(target, { isAdmin: true }), {});
});

test('merge preserves build configuration arrays, symbols and custom merges', () => {
	const key = Symbol('config');
	const original = { nested: { first: 1 }, plugins: ['first'], [key]: { first: true } };
	const merged = merge(original, {
		nested: { second: 2 },
		plugins: ['second'],
		[key]: { second: true }
	});
	assert.deepEqual(merged, {
		nested: { first: 1, second: 2 },
		plugins: ['first', 'second'],
		[key]: { first: true, second: true }
	});
	assert.deepEqual(original.nested, { first: 1 });
	assert.deepEqual(
		merge(
			{ plugins: ['first'] },
			{ plugins: ['second'] },
			{ arrayMerge: (_target, source) => source }
		),
		{ plugins: ['second'] }
	);
	assert.deepEqual(
		merge(
			{ nested: { first: 1 } },
			{ nested: { second: 2 } },
			{ customMerge: (key) => (key === 'nested' ? (_target, source) => source : undefined) }
		),
		{ nested: { second: 2 } }
	);
	assert.deepEqual(merge.all([{ one: 1 }, { two: 2 }]), { one: 1, two: 2 });
	const date = new Date(0);
	assert.equal(merge({}, { date }).date, date);
	const child = { value: 1 };
	assert.equal(merge({}, { child }, { clone: false }).child, child);
});
