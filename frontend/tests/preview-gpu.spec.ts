import { expect, test } from './fixtures';

test('GPU palette agrees with exported colors for axes, zero and oblique vectors', async ({
	page
}) => {
	const errors: string[] = [];
	page.on('pageerror', (error) => errors.push(error.message));
	await page.goto('/');
	const comparisons = await page.evaluate(async () => {
		const threePath = '/node_modules/.vite/deps/three.js';
		const THREE = await import(threePath);
		const meshPath = '/src/lib/preview/vectorInstances.ts';
		const { VectorMesh } = await import(meshPath);
		const colorsPath = '/src/lib/preview/previewColors.ts';
		const { vectorOrientationColor } = await import(colorsPath);
		const renderer = new THREE.WebGLRenderer({ antialias: false });
		renderer.setSize(32, 32);
		const target = new THREE.WebGLRenderTarget(32, 32);
		target.texture.colorSpace = THREE.SRGBColorSpace;
		const scene = new THREE.Scene(),
			camera = new THREE.OrthographicCamera(-1, 1, 1, -1, 0.1, 10);
		camera.position.z = 3;
		const mesh = new VectorMesh(new THREE.BoxGeometry(1, 1, 1), new THREE.MeshBasicMaterial(), 1);
		mesh.uniforms.previewMode.value = 1;
		mesh.count = 1;
		mesh.frustumCulled = false;
		scene.add(mesh);
		const results = [];
		for (const vector of [
			[1, 0, 0],
			[-1, 0, 0],
			[0, 1, 0],
			[0, -1, 0],
			[0, 0, 1],
			[0, 0, -1],
			[0, 0, 0],
			[0.3, -0.4, 0.5]
		]) {
			mesh.vectors.array.set(vector);
			mesh.upload(true);
			renderer.setRenderTarget(target);
			renderer.render(scene, camera);
			const pixel = new Uint8Array(4);
			renderer.readRenderTargetPixels(target, 16, 16, 1, 1, pixel);
			const expected = vectorOrientationColor(...vector);
			results.push({
				actual: Array.from(pixel.slice(0, 3)),
				expected: [expected.r, expected.g, expected.b].map((v) => Math.round(v * 255))
			});
		}
		mesh.dispose();
		target.dispose();
		renderer.dispose();
		return results;
	});
	for (const comparison of comparisons)
		for (let i = 0; i < 3; i++)
			expect(Math.abs(comparison.actual[i] - comparison.expected[i])).toBeLessThanOrEqual(1);
	expect(errors).toEqual([]);
});
