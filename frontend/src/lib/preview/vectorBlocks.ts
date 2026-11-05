export type YieldTask = () => Promise<void>;

export async function buildVectorBlocks(
	itemCount: number,
	blockCount: number,
	blockForItem: (index: number) => number,
	shouldContinue: () => boolean,
	yieldTask: YieldTask,
	chunkSize = 65_536
): Promise<Uint32Array[] | null> {
	const count = Math.max(0, Math.floor(itemCount));
	const groups = Math.max(0, Math.floor(blockCount));
	if (groups === 0) return [];
	const average = Math.ceil(count / groups);
	const initialCapacity = count === 0 ? 0 : Math.min(count, Math.max(64, average));
	const buffers = Array.from({ length: groups }, () => new Uint32Array(initialCapacity));
	const lengths = new Uint32Array(groups);
	const safeChunkSize = Math.max(1, Math.floor(chunkSize));

	for (let index = 0; index < count; index++) {
		if (index > 0 && index % safeChunkSize === 0) {
			await yieldTask();
			if (!shouldContinue()) return null;
		}
		const group = blockForItem(index);
		if (group < 0 || group >= groups) continue;
		const position = lengths[group];
		if (position === buffers[group].length) {
			const capacity = Math.min(count, Math.max(64, buffers[group].length * 2));
			const expanded = new Uint32Array(capacity);
			expanded.set(buffers[group]);
			buffers[group] = expanded;
		}
		buffers[group][position] = index;
		lengths[group] = position + 1;
	}

	const result: Uint32Array[] = [];
	for (let group = 0; group < groups; group++) {
		if (lengths[group] > 0) result.push(buffers[group].subarray(0, lengths[group]));
	}
	return result;
}
