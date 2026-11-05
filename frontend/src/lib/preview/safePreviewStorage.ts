export type PreviewStorage = Pick<Storage, 'getItem' | 'setItem'>;
export type PreviewStorageResolver = () => PreviewStorage | null | undefined;

function browserStorage(): PreviewStorage | null {
	try {
		return typeof window === 'undefined' ? null : window.localStorage;
	} catch {
		return null;
	}
}

export function readPreviewStorage(
	key: string,
	resolveStorage: PreviewStorageResolver = browserStorage
): string | null {
	try {
		return resolveStorage()?.getItem(key) ?? null;
	} catch {
		return null;
	}
}

export function writePreviewStorage(
	key: string,
	value: string,
	resolveStorage: PreviewStorageResolver = browserStorage
): boolean {
	try {
		const storage = resolveStorage();
		if (!storage) return false;
		storage.setItem(key, value);
		return true;
	} catch {
		return false;
	}
}
