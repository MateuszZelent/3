import type { Mesh } from '$api/incoming/mesh';

export function previewFrameMesh(frame: { mesh?: Mesh }, currentMesh: Mesh): Mesh {
	return frame.mesh ?? currentMesh;
}
