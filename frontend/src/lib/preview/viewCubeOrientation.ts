import * as THREE from 'three';

// The rendered simulation coordinates are mapped to world (x, z, y).
const simulationToWorld = new THREE.Matrix4().set(1, 0, 0, 0, 0, 0, 1, 0, 0, 1, 0, 0, 0, 0, 0, 1);

// Face labels use CSS cube coordinates: +X is right, +Y is toward the viewer,
// and +Z is screen-up (negative CSS Y).
const simulationToCube = new THREE.Matrix4().set(1, 0, 0, 0, 0, 0, -1, 0, 0, 1, 0, 0, 0, 0, 0, 1);
const worldToCss = new THREE.Matrix4().makeScale(1, -1, 1);

export function viewCubeRotationMatrix(cameraQuaternion: THREE.Quaternion): THREE.Matrix4 {
	const worldToCamera = new THREE.Matrix4().makeRotationFromQuaternion(cameraQuaternion).invert();
	return worldToCss
		.clone()
		.multiply(worldToCamera)
		.multiply(simulationToWorld)
		.multiply(simulationToCube.clone().invert());
}

export function viewCubeCssMatrix(cameraQuaternion: THREE.Quaternion): string {
	return `matrix3d(${viewCubeRotationMatrix(cameraQuaternion).elements.join(',')})`;
}

export function simulationDirectionOnCube(
	cameraQuaternion: THREE.Quaternion,
	direction: THREE.Vector3
): THREE.Vector3 {
	const cubeDirection = new THREE.Vector3(direction.x, -direction.z, direction.y);
	return cubeDirection.applyMatrix4(viewCubeRotationMatrix(cameraQuaternion));
}
