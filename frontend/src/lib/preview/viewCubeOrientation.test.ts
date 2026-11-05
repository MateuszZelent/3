import { describe, expect, it } from 'vitest';
import * as THREE from 'three';
import {
	simulationDirectionOnCube,
	viewCubeCssMatrix,
	viewCubeRotationMatrix
} from './viewCubeOrientation';

const axisViews: Array<[string, THREE.Vector3]> = [
	['+X', new THREE.Vector3(1, 0, 0)],
	['-X', new THREE.Vector3(-1, 0, 0)],
	['+Y', new THREE.Vector3(0, 1, 0)],
	['-Y', new THREE.Vector3(0, -1, 0)],
	['+Z', new THREE.Vector3(0, 0, 1)],
	['-Z', new THREE.Vector3(0, 0, -1)]
];

function cameraForSimulationView(direction: THREE.Vector3) {
	const worldDirection = new THREE.Vector3(direction.x, direction.z, direction.y).normalize();
	const camera = new THREE.PerspectiveCamera();
	camera.position.copy(worldDirection);
	camera.up.set(0, 1, 0);
	if (Math.abs(worldDirection.y) > 0.9) {
		camera.up.set(0, 0, worldDirection.y > 0 ? 1 : -1);
	}
	camera.lookAt(0, 0, 0);
	camera.updateMatrixWorld(true);
	return camera;
}

describe('ViewCube camera basis', () => {
	it.each(axisViews)(
		'puts simulation %s toward the viewer for its six axis views',
		(_name, direction) => {
			const camera = cameraForSimulationView(direction);
			const screenDirection = simulationDirectionOnCube(camera.quaternion, direction);
			expect(screenDirection.x).toBeCloseTo(0, 6);
			expect(screenDirection.y).toBeCloseTo(0, 6);
			expect(screenDirection.z).toBeCloseTo(1, 6);
		}
	);

	it('keeps roll in the full camera quaternion', () => {
		const view = new THREE.Vector3(0, 0, 1);
		const camera = cameraForSimulationView(view);
		const before = viewCubeCssMatrix(camera.quaternion);
		const worldAxis = new THREE.Vector3(0, 1, 0);
		const rolled = new THREE.Quaternion()
			.setFromAxisAngle(worldAxis, Math.PI / 3)
			.multiply(camera.quaternion);
		const after = viewCubeCssMatrix(rolled);
		expect(after).not.toBe(before);
		expect(simulationDirectionOnCube(rolled, view).z).toBeCloseTo(1, 6);
		const matrix = viewCubeRotationMatrix(rolled);
		expect(new THREE.Vector3(0, 1, 0).applyMatrix4(matrix).length()).toBeCloseTo(1, 6);
	});
});
