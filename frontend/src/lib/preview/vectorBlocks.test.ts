import { describe, expect, it } from 'vitest';
import { buildVectorBlocks } from './vectorBlocks';

describe('bounded vector topology grouping', () => {
	it('keeps source order while using bounded typed index blocks', async () => {
		const blocks = await buildVectorBlocks(
			11,
			3,
			(index) => index % 3,
			() => true,
			async () => undefined,
			3
		);
		expect(blocks?.map((block) => Array.from(block))).toEqual([
			[0, 3, 6, 9],
			[1, 4, 7, 10],
			[2, 5, 8]
		]);
	});

	it('cancels at a yield boundary without returning partial topology', async () => {
		let active = true;
		const blocks = await buildVectorBlocks(
			8,
			2,
			(index) => index % 2,
			() => active,
			async () => {
				active = false;
			},
			2
		);
		expect(blocks).toBeNull();
	});
});
