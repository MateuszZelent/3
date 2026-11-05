import { expect, publishPreviewConfig, test } from './fixtures';
import type { Preview } from '../src/api/incoming/preview';
import type { PreviewRegion } from '../src/lib/preview/preview3DRegion';
import type { PreviewDisplay, VolumeSnapshot } from './preview-test-types';

test('Volume closes cells, keeps occupied zeros and preserves cavities and clipping', async ({
	page,
	baseURL
}) => {
	const errors: string[] = [];
	page.on('pageerror', (e) => errors.push(e.message));
	page.on('console', (m) => {
		if (m.type() === 'error' && !m.text().includes('WebSocket')) errors.push(m.text());
	});
	let configRevision = 200;
	const fullPositions: number[] = [];
	const fullValues: number[] = [];
	for (let z = 0; z < 3; z++) {
		for (let y = 0; y < 3; y++) {
			for (let x = 0; x < 3; x++) {
				if (x === 1 && y === 1 && z === 1) continue;
				fullPositions.push(x, y, z);
				fullValues.push(0, 0, 0);
			}
		}
	}
	await page.route(`${baseURL}/api/preview/region`, async (route) => {
		const draft = route.request().postDataJSON() as PreviewRegion;
		const revision = ++configRevision;
		const applied: PreviewRegion = { ...draft, configRevision: revision };
		const bounds = applied.enabled ? applied : { start: [0, 0, 0], end: [3, 3, 3] };
		const croppedPositions: number[] = [];
		const croppedValues: number[] = [];
		for (let i = 0; i < fullPositions.length; i += 3) {
			const x = fullPositions[i],
				y = fullPositions[i + 1],
				z = fullPositions[i + 2];
			if (
				x < bounds.start[0] ||
				x >= bounds.end[0] ||
				y < bounds.start[1] ||
				y >= bounds.end[1] ||
				z < bounds.start[2] ||
				z >= bounds.end[2]
			)
				continue;
			croppedPositions.push(x - bounds.start[0], y - bounds.start[1], z - bounds.start[2]);
			croppedValues.push(fullValues[i], fullValues[i + 1], fullValues[i + 2]);
		}
		await publishPreviewConfig(
			page,
			{
				region: applied,
				regionActive: applied.enabled,
				vectorFieldPositions: new Int32Array(croppedPositions),
				vectorFieldValues: new Float32Array(croppedValues),
				vectorCount: croppedValues.length / 3,
				appliedXChosenSize: bounds.end[0] - bounds.start[0],
				appliedYChosenSize: bounds.end[1] - bounds.start[1],
				appliedZChosenSize: bounds.end[2] - bounds.start[2],
				topologyRevision: revision
			},
			revision
		);
		await route.fulfill({ json: applied });
	});
	await page.goto('/');
	await page.getByRole('button', { name: 'Single layer', exact: true }).waitFor();
	await page.evaluate(async () => {
		const mp = '/src/api/incoming/mesh.ts',
			pp = '/src/api/incoming/preview.ts',
			rp = '/src/lib/preview/preview3D.ts',
			wp = '/src/api/websocket.ts';
		const { meshState } = await import(mp),
			{ previewState } = await import(pp),
			{
				preview3D,
				setRenderMode,
				setVoxelThreshold,
				setVoxelSampling,
				setTopoEnabled,
				setVolumeColorMode
			} = await import(rp),
			{ connected, previewConnected } = await import(wp);
		connected.set(true);
		previewConnected.set(true);
		meshState.set({ Nx: 3, Ny: 3, Nz: 3, dx: 5e-9, dy: 10e-9, dz: 15e-9 });
		const positions: number[] = [],
			values: number[] = [];
		for (let z = 0; z < 3; z++)
			for (let y = 0; y < 3; y++)
				for (let x = 0; x < 3; x++) {
					if (x === 1 && y === 1 && z === 1) continue;
					positions.push(x, y, z);
					values.push(0, 0, 0);
				}
		previewState.update((state: Preview) => ({
			...state,
			type: '3D',
			nComp: 3,
			quantity: 'm',
			allLayers: true,
			appliedXChosenSize: 3,
			appliedYChosenSize: 3,
			appliedZChosenSize: 3,
			appliedLayerStride: 1,
			transportSampling: 1,
			vectorFieldPositions: new Int32Array(positions),
			vectorFieldValues: new Float32Array(values),
			vectorCount: 26,
			topologyRevision: 123
		}));
		setVoxelThreshold(0.9);
		setVoxelSampling(4);
		setTopoEnabled(true);
		setVolumeColorMode('geometry');
		setRenderMode('volume');
		await new Promise(requestAnimationFrame);
		await preview3D();
	});
	const read = () =>
		page.evaluate<VolumeSnapshot>(async () => {
			const rp = '/src/lib/preview/preview3D.ts';
			const { threeDPreview, preview3D } = await import(rp);
			await preview3D();
			const snapshot: { value: PreviewDisplay | null } = { value: null };
			threeDPreview.subscribe((value: PreviewDisplay | null) => (snapshot.value = value))();
			const d = snapshot.value;
			if (!d) throw new Error('3D preview has not rendered');
			const mesh = d.mesh;
			return {
				count: mesh.count,
				depths: Array.from(mesh.depths.array.slice(0, mesh.count)),
				scale: mesh.uniforms.previewScale.value.toArray(),
				solid: mesh.uniforms.previewSolid.value,
				topo: mesh.uniforms.previewTopo.value,
				color: mesh.uniforms.previewColorMode.value,
				opaque: !mesh.material.transparent,
				opacity: mesh.material.opacity,
				positions: Array.from(mesh.offsets.array.slice(0, mesh.count * 3)),
				triangles: d.renderer.info.render.triangles
			};
		});
	await expect(page.getByRole('button', { name: 'Volume', exact: true })).toHaveAttribute(
		'aria-pressed',
		'true'
	);
	const data = await read();
	expect(data.count).toBe(26);
	expect(data.opaque).toBe(true);
	expect(data.opacity).toBe(1);
	expect(data.topo).toBe(0);
	expect(data.solid).toBe(1);
	expect(data.scale[1] / data.scale[0]).toBeCloseTo(3);
	expect(data.scale[2] / data.scale[0]).toBeCloseTo(2);
	// Interior absent cell must stay absent, even when occupied zero-field cells are shown.
	const center = [data.scale[0] * 1.5, data.scale[1] * 1.5, data.scale[2] * 1.5];
	const positions = data.positions as number[];
	expect(
		Array.from({ length: 26 }, (_, i) => positions.slice(i * 3, i * 3 + 3)).some((p) =>
			p.every((n, j) => Math.abs(n - center[j]) < 0.001)
		)
	).toBe(false);
	await page.locator('.region-controls > summary').click();
	const croppedRequest = page.waitForRequest((request) =>
		request.url().endsWith('/api/preview/region')
	);
	const regionAccepted = await page.evaluate(async () => {
		const regionPath = '/src/lib/preview/preview3DRegion.ts';
		const { changeRegion } = await import(regionPath);
		return changeRegion({ enabled: true, start: [0, 0, 0], end: [1, 3, 3] });
	});
	expect(regionAccepted).toBe(true);
	expect((await croppedRequest).postDataJSON()).toMatchObject({
		enabled: true,
		start: [0, 0, 0],
		end: [1, 3, 3]
	});
	await expect.poll(async () => (await read()).count).toBe(9);
	await page.getByLabel('Color by', { exact: true }).selectOption('z');
	expect((await read()).solid).toBe(0);
	expect((await read()).color).toBe(3);
	const fullDomainRequest = page.waitForRequest((request) =>
		request.url().endsWith('/api/preview/region')
	);
	await page.evaluate(async () => {
		const regionPath = '/src/lib/preview/preview3DRegion.ts';
		const { fullDomain } = await import(regionPath);
		fullDomain();
	});
	expect((await fullDomainRequest).postDataJSON()).toMatchObject({
		enabled: false,
		start: [0, 0, 0],
		end: [3, 3, 3]
	});
	await expect.poll(async () => (await read()).count).toBe(26);
	for (const width of [1640, 390]) {
		await page.setViewportSize({ width, height: 1100 });
		await page.locator('.preview-wrapper').screenshot({ path: `/tmp/3-solid-${width}.png` });
	}
	await page.getByRole('button', { name: 'Arrows', exact: true }).click();
	await expect.poll(async () => (await read()).count).toBe(0);
	await page.getByRole('button', { name: 'Volume', exact: true }).click();
	await expect.poll(async () => (await read()).count).toBe(26);
	await page.evaluate(async () => {
		const path = '/src/api/incoming/preview.ts',
			renderPath = '/src/lib/preview/preview3D.ts';
		const { previewState } = await import(path),
			{ setVolumeColorMode } = await import(renderPath);
		previewState.update((state: Preview) => ({
			...state,
			normScale: 2,
			vectorFieldValues: new Float32Array(
				Array.from(state.vectorFieldPositions, (value: number, i: number) =>
					i % 3 === 2 ? value - 1 : 0
				)
			)
		}));
		setVolumeColorMode('z');
	});
	const volumeControls = page.locator('.volume-controls');
	if (!(await volumeControls.evaluate((element) => (element as HTMLDetailsElement).open))) {
		await volumeControls.locator('summary').click();
	}
	await expect(page.getByLabel('Surface lighting', { exact: true })).not.toBeChecked();
	const palette = await page.evaluate<{
		pixel: number[];
		range: number[];
		tone: boolean;
		type: string;
	}>(async () => {
		const p = '/src/lib/preview/preview3D.ts',
			tp = '/node_modules/.vite/deps/three.js';
		const { threeDPreview, preview3D } = await import(p),
			{ Vector3 } = await import(tp);
		await preview3D();
		const snapshot: { value: PreviewDisplay | null } = { value: null };
		threeDPreview.subscribe((value: PreviewDisplay | null) => (snapshot.value = value))();
		const d = snapshot.value;
		if (!d) throw new Error('3D preview has not rendered');
		const mesh = d.mesh,
			offsets = mesh.offsets.array,
			scale = mesh.uniforms.previewScale.value;
		const point = new Vector3(
			offsets[18 * 3],
			offsets[18 * 3 + 1] + scale.y / 2,
			offsets[18 * 3 + 2]
		).project(d.camera);
		d.renderer.render(d.scene, d.camera);
		const gl = d.renderer.getContext(),
			pixel = new Uint8Array(4);
		gl.readPixels(
			Math.floor((point.x * 0.5 + 0.5) * gl.drawingBufferWidth),
			Math.floor((point.y * 0.5 + 0.5) * gl.drawingBufferHeight),
			1,
			1,
			gl.RGBA,
			gl.UNSIGNED_BYTE,
			pixel
		);
		return {
			pixel: Array.from(pixel),
			range: mesh.uniforms.previewRange.value.toArray(),
			tone: mesh.material.toneMapped,
			type: mesh.material.type
		};
	});
	expect(palette.range).toEqual([-2, 2]);
	expect(palette.tone).toBe(false);
	expect(palette.type).toBe('MeshBasicMaterial');
	for (const [i, expected] of [125, 29, 52].entries())
		expect(Math.abs(palette.pixel[i] - expected)).toBeLessThan(3);
	await page.evaluate(async () => {
		const p = '/src/lib/preview/preview3D.ts';
		const { setVolumeProjection } = await import(p);
		setVolumeProjection('average', 'z');
	});
	await expect
		.poll(async () =>
			page.evaluate<boolean>(async () => {
				const p = '/src/lib/preview/preview3D.ts';
				const { threeDPreview, preview3D } = await import(p);
				await preview3D();
				const snapshot: { value: PreviewDisplay | null } = { value: null };
				threeDPreview.subscribe((value: PreviewDisplay | null) => (snapshot.value = value))();
				const d = snapshot.value;
				if (!d) throw new Error('3D preview has not rendered');
				return Array.from(d.mesh.vectors.array.slice(0, d.mesh.count * 3)).every(
					(value) => Math.abs(value) < 0.00001
				);
			})
		)
		.toBe(true);
	await page.evaluate(async () => {
		const p = '/src/lib/preview/preview3D.ts';
		const { setVolumeProjection } = await import(p);
		setVolumeProjection('average', 'x');
	});
	await expect
		.poll(async () =>
			page.evaluate<number>(async () => {
				const p = '/src/lib/preview/preview3D.ts';
				const { threeDPreview, preview3D } = await import(p);
				await preview3D();
				const snapshot: { value: PreviewDisplay | null } = { value: null };
				threeDPreview.subscribe((value: PreviewDisplay | null) => (snapshot.value = value))();
				const d = snapshot.value;
				if (!d) throw new Error('3D preview has not rendered');
				return d.mesh.vectors.array[2];
			})
		)
		.toBe(-1);
	await page.getByRole('button', { name: 'Manual', exact: true }).click();
	await page.getByLabel('3D color minimum').fill('-4');
	await page.getByLabel('3D color maximum').fill('2');
	expect(
		await page.evaluate<number[]>(async () => {
			const p = '/src/lib/preview/preview3D.ts';
			const { threeDPreview, preview3D } = await import(p);
			await preview3D();
			const snapshot: { value: PreviewDisplay | null } = { value: null };
			threeDPreview.subscribe((value: PreviewDisplay | null) => (snapshot.value = value))();
			const d = snapshot.value;
			if (!d) throw new Error('3D preview has not rendered');
			return d.mesh.uniforms.previewRange.value.toArray();
		})
	).toEqual([-4, 2]);
	await page.locator('.volume-controls').screenshot({ path: '/tmp/3-solid-controls-mobile.png' });
	await page.evaluate(async () => {
		const p = '/src/api/incoming/preview.ts';
		const { previewState } = await import(p);
		previewState.update((state: Preview) => ({
			...state,
			nComp: 1,
			quantity: 'geom',
			vectorOccupancy: new Uint8Array(state.vectorCount).fill(1)
		}));
	});
	await page.getByLabel('Color by', { exact: true }).selectOption('value');
	await expect(page.getByRole('button', { name: 'Arrows', exact: true })).toBeDisabled();
	await expect(page.getByRole('button', { name: 'Voxel', exact: true })).toBeDisabled();
	expect((await read()).color).toBe(1);
	await page.evaluate(async () => {
		const p = '/src/api/incoming/preview.ts';
		const { previewState } = await import(p);
		previewState.update((state: Preview) => ({
			...state,
			vectorOccupancy: new Uint8Array(state.vectorCount).fill(0)
		}));
	});
	await expect.poll(async () => (await read()).count).toBe(0);
	await page.evaluate(async () => {
		const p = '/src/api/incoming/preview.ts',
			mp = '/src/api/incoming/mesh.ts',
			r = '/src/lib/preview/preview3D.ts';
		const { previewState } = await import(p),
			{ meshState } = await import(mp),
			{ setVolumeColorMode, setVolumeProjection } = await import(r);
		meshState.set({ Nx: 1, Ny: 1, Nz: 3, dx: 1e-9, dy: 1e-9, dz: 1e-9 });
		previewState.update((state: Preview) => ({
			...state,
			nComp: 3,
			quantity: 'm',
			normScale: 1,
			allLayers: true,
			appliedXChosenSize: 1,
			appliedYChosenSize: 1,
			appliedZChosenSize: 2,
			appliedLayerStride: 2,
			vectorFieldPositions: new Int32Array([0, 0, 1, 0, 0, 2]),
			vectorFieldValues: new Float32Array([1, 0, 0, 0, 0, 0]),
			vectorCount: 2,
			vectorOccupancy: undefined,
			topologyRevision: 124
		}));
		setVolumeColorMode('x');
		setVolumeProjection('average', 'z');
	});
	const partial = await read();
	expect(partial.depths).toEqual([1, 0.5]);
	expect(partial.positions[4] + (partial.scale[1] * partial.depths[1]) / 2).toBeCloseTo(100);
	const average = await page.evaluate<number>(async () => {
		const p = '/src/lib/preview/preview3D.ts';
		const { threeDPreview, preview3D } = await import(p);
		await preview3D();
		const snapshot: { value: PreviewDisplay | null } = { value: null };
		threeDPreview.subscribe((value: PreviewDisplay | null) => (snapshot.value = value))();
		const d = snapshot.value;
		if (!d) throw new Error('3D preview has not rendered');
		return d.mesh.vectors.array[0];
	});
	expect(average).toBeCloseTo(2 / 3);
	expect(errors).toEqual([]);
});
