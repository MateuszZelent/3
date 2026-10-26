import { describe, expect, it } from 'vitest';
import * as THREE from 'three';
import { VectorMesh } from './vectorInstances';

describe('VectorMesh instance count', () => {
	it('keeps the public count and WebGL geometry count synchronized', () => {
		const mesh = new VectorMesh(new THREE.BoxGeometry(), new THREE.MeshBasicMaterial(), 4);
		expect(mesh.count).toBe(0);
		expect(mesh.geometry.instanceCount).toBe(0);
		mesh.count = 3;
		expect(mesh.count).toBe(3);
		expect(mesh.geometry.instanceCount).toBe(3);
		mesh.geometry.instanceCount = 1;
		expect(mesh.count).toBe(1);
		mesh.dispose();
	});
});
