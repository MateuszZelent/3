import { browser } from '$app/environment';
import { previewState } from '$api/incoming/preview';
import { meshState } from '$api/incoming/mesh';
import { setPreviewClientBudget, previewClientBudget } from '$api/websocket';
import * as THREE from 'three';
import * as BufferGeometryUtils from 'three/examples/jsm/utils/BufferGeometryUtils.js';
import { TrackballControls } from 'three/examples/jsm/controls/TrackballControls.js';
import { get, writable } from 'svelte/store';
import { disposePreview2D } from './preview2D';
import { preview3DLayout } from './preview3DLayout';
import { projectVolumeVectors, type ProjectionAxis } from './volumeProjection';
import { getColorScale } from './fieldColorScale';
import { postAllLayers } from '$api/outgoing/preview';
import { VectorMesh } from './vectorInstances';
import { THEME } from '$lib/theme/echarts-theme';

export type QualityLevel = 'low' | 'high' | 'ultra';
export type Preview3DRenderMode = 'glyph' | 'voxel' | 'volume';
export type VoxelColorMode = 'orientation' | 'magnitude' | 'value' | 'x' | 'y' | 'z';
export type VolumeColorMode = 'geometry' | 'value' | VoxelColorMode;
export const volumeColorMode = writable<VolumeColorMode>('orientation');
export const volumeProjection = writable<'surface' | 'average'>('surface');
export const volumeProjectionAxis = writable<ProjectionAxis>('z');
export const volumeLighting = writable(false);
export const volumeScaleMode = writable<'data' | 'manual'>('data');
export const volumeManualRange = writable<[number, number]>([-1, 1]);
export const volumeLegend = writable({ min: 0, max: 1, palette: ['#0a1220', '#f1f7bb'] });
let projectedSource: Float32Array | null = null;
let projectedPositions: Int32Array | null = null;
let projectedKey = '';
let projectedValues: Float32Array = new Float32Array();
export type VoxelSampling = 1 | 2 | 4;
export type TopoComponent = 'x' | 'y' | 'z';

interface QualityConfig {
	segments: number;
	useLighting: boolean;
	useHemisphere: boolean;
	antialias: boolean;
	pixelRatio: number;
}

interface ThreeDPreview {
	mesh: VectorMesh;
	meshes: VectorMesh[];
	blocks: Uint32Array[];
	topologyKey: string;
	lastValues: Float32Array | null;
	lastOccupancy?: Uint8Array;
	filterKey: string;
	liveIndices: Uint32Array[];
	meshCapacity: number;
	scene: THREE.Scene;
	camera: THREE.PerspectiveCamera;
	renderer: THREE.WebGLRenderer;
	controls: TrackballControls;
	rendererMode: Preview3DRenderMode;
	shapeKey: string;
}

const QUALITY_CONFIGS: Record<QualityLevel, QualityConfig> = {
	low: {
		segments: 6,
		useLighting: false,
		useHemisphere: false,
		antialias: false,
		pixelRatio: 1
	},
	high: {
		segments: 12,
		useLighting: true,
		useHemisphere: true,
		antialias: true,
		pixelRatio: 1
	},
	ultra: {
		segments: 16,
		useLighting: true,
		useHemisphere: true,
		antialias: true,
		pixelRatio: browser ? Math.min(window.devicePixelRatio, 2) : 1
	}
};

const STORAGE_KEYS = {
	brightness: 'preview3d_brightness',
	quality: 'preview3d_quality',
	renderMode: 'preview3d_render_mode',
	voxelOpacity: 'preview3d_voxel_opacity',
	voxelGap: 'preview3d_voxel_gap',
	voxelThreshold: 'preview3d_voxel_threshold',
	voxelColorMode: 'preview3d_voxel_color_mode',
	voxelSampling: 'preview3d_voxel_sampling',
	topoEnabled: 'preview3d_topo_enabled',
	topoComponent: 'preview3d_topo_component',
	topoMultiplier: 'preview3d_topo_multiplier'
} as const;

const _tempVec = new THREE.Vector3();
const _camDir = new THREE.Vector3();

// Local coordinates avoid precision loss for a small window far from the origin.
// Source bounds remain available in the window controls in physical units.
function getRenderMesh() {
	const mesh = get(meshState),
		p = get(previewState),
		r = p.region;
	if (!p.regionActive || !r) return mesh;
	const nz = Math.max(p.appliedZChosenSize, 1);
	return {
		...mesh,
		Nx: r.end[0] - r.start[0],
		Ny: r.end[1] - r.start[1],
		Nz: nz,
		dz: ((r.end[2] - r.start[2]) * mesh.dz) / nz
	};
}
function getLayout() {
	const preview = get(previewState);
	return preview3DLayout(
		getRenderMesh(),
		getPreviewWidthCells(),
		getPreviewHeightCells(),
		preview.appliedLayerStride || 1,
		preview.allLayers
	);
}

function getWorldExtents() {
	const { xSize, ySize, depthCells } = getLayout();
	return {
		xSize,
		ySize,
		depthCells,
		centerX: xSize / 2,
		centerY: depthCells / 2,
		centerZ: ySize / 2,
		orbitDistance: Math.max(xSize, ySize, depthCells) * 1.5
	};
}

function getPreviewShapeKey() {
	const { xSize, ySize, depthCells } = getLayout();
	return `${xSize}x${ySize}x${depthCells}:${get(previewState).allLayers}`;
}

function nextMeshCapacity(required: number) {
	let capacity = 1;
	while (capacity < required) {
		capacity *= 2;
	}
	return capacity;
}

function setWorldDirectionFromSimulation(
	target: THREE.Vector3,
	direction: { x: number; y: number; z: number }
) {
	return target.set(direction.x, direction.z, direction.y);
}

export const brightness = writable<number>(loadBrightness());
export const qualityLevel = writable<QualityLevel>(loadQuality());
export const renderMode = writable<Preview3DRenderMode>(loadRenderMode());
export const voxelOpacity = writable<number>(
	loadClampedNumber(STORAGE_KEYS.voxelOpacity, 0.5, 0.15, 0.95)
);
export const voxelGap = writable<number>(
	loadClampedNumber(STORAGE_KEYS.voxelGap, 0.14, 0.02, 0.42)
);
export const voxelThreshold = writable<number>(
	loadClampedNumber(STORAGE_KEYS.voxelThreshold, 0.08, 0, 0.95)
);
export const voxelColorMode = writable<VoxelColorMode>(loadVoxelColorMode());
export const voxelSampling = writable<VoxelSampling>(loadVoxelSampling());
export const threeDPreview = writable<ThreeDPreview | null>(null);
export const voxelOpaque = writable(
	browser ? localStorage.getItem('preview3d_opaque') === 'true' : false
);
export const glyphSampling = writable<VoxelSampling>(1);
export const previewPerformance = writable({
	updateMs: 0,
	renderMs: 0,
	drawCalls: 0,
	triangles: 0,
	uploadBytes: 0,
	lod: false
});
export const visibleRenderCount = writable<number>(0);
export const cameraRevision = writable<number>(0);
export const topoEnabled = writable<boolean>(loadTopoEnabled());
export const topoComponent = writable<TopoComponent>(loadTopoComponent());
export const topoMultiplier = writable<number>(
	loadClampedNumber(STORAGE_KEYS.topoMultiplier, 5, 0.5, 50)
);

let renderFrameId: number | null = null;
let controlsActive = false;
let resizeObserver: ResizeObserver | null = null;

function loadClampedNumber(key: string, fallback: number, min: number, max: number) {
	if (!browser) {
		return fallback;
	}

	const raw = Number.parseFloat(window.localStorage.getItem(key) || '');
	if (!Number.isFinite(raw)) {
		return fallback;
	}

	return Math.max(min, Math.min(max, raw));
}

function loadBrightness() {
	return loadClampedNumber(STORAGE_KEYS.brightness, 1.5, 0.3, 3.0);
}

function loadQuality(): QualityLevel {
	if (!browser) {
		return 'high';
	}

	const stored = window.localStorage.getItem(STORAGE_KEYS.quality);
	if (stored && stored in QUALITY_CONFIGS) {
		return stored as QualityLevel;
	}

	return 'high';
}

function loadRenderMode(): Preview3DRenderMode {
	if (!browser) {
		return 'glyph';
	}

	const stored = window.localStorage.getItem(STORAGE_KEYS.renderMode);
	return stored === 'voxel' || stored === 'volume' ? stored : 'glyph';
}

function loadVoxelColorMode(): VoxelColorMode {
	if (!browser) {
		return 'orientation';
	}

	const stored = window.localStorage.getItem(STORAGE_KEYS.voxelColorMode);
	if (
		stored === 'x' ||
		stored === 'y' ||
		stored === 'z' ||
		stored === 'magnitude' ||
		stored === 'value'
	) {
		return stored;
	}

	return 'orientation';
}

function loadVoxelSampling(): VoxelSampling {
	if (!browser) {
		return 1;
	}

	const stored = Number.parseInt(window.localStorage.getItem(STORAGE_KEYS.voxelSampling) || '', 10);
	return stored === 2 || stored === 4 ? stored : 1;
}

function loadTopoEnabled(): boolean {
	if (!browser) return false;
	return window.localStorage.getItem(STORAGE_KEYS.topoEnabled) === 'true';
}

function loadTopoComponent(): TopoComponent {
	if (!browser) return 'z';
	const stored = window.localStorage.getItem(STORAGE_KEYS.topoComponent);
	if (stored === 'x' || stored === 'y' || stored === 'z') return stored;
	return 'z';
}

function persistSetting(key: string, value: string | number) {
	if (!browser) {
		return;
	}

	window.localStorage.setItem(key, String(value));
}

function has3DPreviewData() {
	const state = get(previewState);
	return state.type === '3D' && (state.nComp === 3 || state.nComp === 1);
}

function rebuildPreview3D() {
	if (!has3DPreviewData()) {
		return;
	}

	const display = get(threeDPreview);
	if (!display) return;
	// Preserve the canvas/camera; the highest quality context enables MSAA once.
	display.topologyKey = '';
	display.scene.children
		.filter((child) => child instanceof THREE.Light)
		.forEach((child) => display.scene.remove(child));
	const lights = createScene();
	lights.children.slice().forEach((child) => display.scene.add(child));
	display.renderer.setPixelRatio(getConfig().pixelRatio);
	update();
}

function getConfig(): QualityConfig {
	return QUALITY_CONFIGS[get(qualityLevel)];
}

function getPreviewWidthCells() {
	const state = get(previewState);
	return Math.max(state.appliedXChosenSize || state.xChosenSize, 1);
}

function getPreviewHeightCells() {
	const state = get(previewState);
	return Math.max(state.appliedYChosenSize || state.yChosenSize, 1);
}

function componentValue(
	x: number,
	y: number,
	z: number,
	mode: Exclude<VoxelColorMode, 'orientation'>
) {
	switch (mode) {
		case 'magnitude':
			return Math.hypot(x, y, z);
		case 'value':
		case 'x':
			return x;
		case 'y':
			return y;
		case 'z':
			return z;
	}
}

function glyphSegments() {
	const display = get(threeDPreview);
	if (!display) return get(previewState).vectorCount > 200000 ? 6 : getConfig().segments;
	const distance = display.camera.position.distanceTo(display.controls.target);
	const pixels =
		(getLayout().glyphScale * display.renderer.domElement.clientHeight) /
		(2 * Math.tan(THREE.MathUtils.degToRad(display.camera.fov / 2)) * Math.max(distance, 1));
	return pixels < 8 ? 3 : pixels < 20 ? 6 : getConfig().segments;
}
function createArrowGeometry() {
	const seg = glyphSegments();
	const shaftGeometry = new THREE.CylinderGeometry(0.05, 0.05, 0.55, seg);
	shaftGeometry.translate(0, -0.06, 0);
	const headGeometry = new THREE.ConeGeometry(0.2, 0.4, seg);
	headGeometry.translate(0, 0.4, 0);
	const arrowGeometry = BufferGeometryUtils.mergeGeometries([shaftGeometry, headGeometry]);

	if (!arrowGeometry) {
		throw new Error('Could not create arrow geometry');
	}

	shaftGeometry.dispose();
	headGeometry.dispose();
	arrowGeometry.computeVertexNormals();
	return arrowGeometry;
}

function createVoxelGeometry() {
	return new THREE.BoxGeometry(1, 1, 1);
}

function createMaterial(mode: Preview3DRenderMode): THREE.Material {
	const cfg = getConfig();

	if (mode === 'volume') {
		return get(volumeColorMode) === 'geometry' || get(volumeLighting)
			? new THREE.MeshPhongMaterial({ shininess: 18, specular: new THREE.Color(0x24334c) })
			: new THREE.MeshBasicMaterial({ toneMapped: false });
	}
	if (mode === 'voxel') {
		if (cfg.useLighting) {
			return new THREE.MeshPhongMaterial({
				transparent: !get(voxelOpaque),
				opacity: get(voxelOpacity),
				depthWrite: get(voxelOpaque),
				shininess: 24,
				specular: new THREE.Color(0x24334c)
			});
		}

		return new THREE.MeshBasicMaterial({
			transparent: !get(voxelOpaque),
			opacity: get(voxelOpacity),
			depthWrite: get(voxelOpaque)
		});
	}

	if (cfg.useLighting) {
		return new THREE.MeshPhongMaterial({
			shininess: 60,
			specular: new THREE.Color(0x444444)
		});
	}

	return new THREE.MeshBasicMaterial();
}

function createMesh(requiredCapacity = 1): VectorMesh {
	const mode = get(renderMode);
	const base = mode !== 'glyph' ? createVoxelGeometry() : createArrowGeometry();
	const mesh = new VectorMesh(
		base,
		createMaterial(mode),
		nextMeshCapacity(Math.max(requiredCapacity, 1))
	);
	mesh.renderOrder = mode === 'voxel' ? 2 : 1;
	return mesh;
}

function createCamera() {
	const container = document.getElementById('container');
	const width = container?.offsetWidth || 1;
	const height = container?.offsetHeight || 1;
	const camera = new THREE.PerspectiveCamera(50, width / height, 0.1, 1000);
	setDefaultCameraPose(camera);
	return camera;
}

function setDefaultCameraPose(camera: THREE.PerspectiveCamera) {
	const { centerX, centerY, centerZ, orbitDistance } = getWorldExtents();

	// Simulation Z maps to world Y, so looking down the +Z simulation axis
	// presents the film's XY plane. Keep simulation +Y pointing upward.
	camera.position.set(centerX, centerY + orbitDistance, centerZ);
	camera.up.set(0, 0, 1);
	camera.lookAt(centerX, centerY, centerZ);
}

function createRenderer() {
	const cfg = getConfig();
	const renderer = new THREE.WebGLRenderer({
		antialias: true,
		alpha: false
	});

	renderer.setPixelRatio(cfg.pixelRatio);
	renderer.setClearColor(THEME.bg, 1);
	renderer.toneMapping = THREE.ACESFilmicToneMapping;
	renderer.toneMappingExposure = 1.05;

	const container = document.getElementById('container');
	if (!container) {
		throw new Error('Container not found');
	}

	renderer.setSize(container.clientWidth, container.clientHeight);
	container.appendChild(renderer.domElement);
	return renderer;
}

function createControls(camera: THREE.PerspectiveCamera, renderer: THREE.WebGLRenderer) {
	const controls = new TrackballControls(camera, renderer.domElement);
	const { centerX, centerY, centerZ } = getWorldExtents();

	controls.dynamicDampingFactor = 1;
	controls.panSpeed = 0.8;
	controls.rotateSpeed = 1;
	controls.target.set(centerX, centerY, centerZ);
	controls.update();
	installCameraRoll(camera, renderer.domElement, controls);

	return controls;
}

function installCameraRoll(
	camera: THREE.PerspectiveCamera,
	canvas: HTMLCanvasElement,
	controls: TrackballControls
) {
	let rolling = false;
	let pointerId = 0;
	let lastX = 0;
	let previousFlags = { enabled: true, noRotate: false, noPan: false, noZoom: false };
	const axis = new THREE.Vector3();
	canvas.title =
		'Left drag: orbit; right drag: pan; left + right drag horizontally: roll; wheel: zoom';

	function endRoll(event?: MouseEvent) {
		if (!rolling) return;
		rolling = false;
		if (canvas.hasPointerCapture(pointerId)) canvas.releasePointerCapture(pointerId);
		Object.assign(controls, previousFlags);
		controls.dispatchEvent({ type: 'end' });
		// Restart the remaining single-button gesture at the current position.
		if (event && (event.buttons & 3) !== 0) {
			canvas.dispatchEvent(
				new PointerEvent('pointerdown', {
					pointerId,
					pointerType: 'mouse',
					button: event.buttons & 1 ? 0 : 2,
					buttons: event.buttons,
					clientX: event.clientX,
					clientY: event.clientY
				})
			);
		}
	}

	function onPointerMove(event: PointerEvent) {
		if (!rolling || event.pointerType !== 'mouse') return;
		if ((event.buttons & 3) !== 3) {
			endRoll(event);
			return;
		}
		event.preventDefault();
		event.stopImmediatePropagation();
		const angle = ((event.clientX - lastX) * 2 * Math.PI) / Math.max(canvas.clientWidth, 1);
		lastX = event.clientX;
		camera.getWorldDirection(axis);
		camera.up.applyAxisAngle(axis, angle).normalize();
		camera.lookAt(controls.target);
		controls.dispatchEvent({ type: 'change' });
	}

	function onPointerDown(event: PointerEvent) {
		if (event.pointerType === 'mouse') pointerId = event.pointerId;
	}

	function onMouseDown(event: MouseEvent) {
		if (rolling || !controls.enabled || (event.buttons & 3) !== 3) return;
		// End TrackballControls' existing orbit/pan gesture before starting roll.
		controls.update();
		canvas.dispatchEvent(new PointerEvent('pointerup', { pointerId, pointerType: 'mouse' }));
		previousFlags = {
			enabled: controls.enabled,
			noRotate: controls.noRotate,
			noPan: controls.noPan,
			noZoom: controls.noZoom
		};
		rolling = true;
		lastX = event.clientX;
		controls.enabled = false;
		controls.noRotate = controls.noPan = controls.noZoom = true;
		canvas.setPointerCapture(pointerId);
		controls.dispatchEvent({ type: 'start' });
		event.preventDefault();
	}

	function onMouseUp(event: MouseEvent) {
		if ((event.buttons & 3) !== 3) endRoll(event);
	}
	function cancelRoll() {
		endRoll();
	}
	function onLostPointerCapture() {
		if (!canvas.hasPointerCapture(pointerId)) cancelRoll();
	}

	canvas.addEventListener('pointerdown', onPointerDown, true);
	canvas.addEventListener('mousedown', onMouseDown, true);
	canvas.addEventListener('pointermove', onPointerMove, true);
	canvas.addEventListener('pointercancel', cancelRoll);
	canvas.addEventListener('lostpointercapture', onLostPointerCapture);
	window.addEventListener('mouseup', onMouseUp);
	window.addEventListener('blur', cancelRoll);
	const dispose = controls.dispose.bind(controls);
	controls.dispose = () => {
		cancelRoll();
		canvas.removeEventListener('pointerdown', onPointerDown, true);
		canvas.removeEventListener('mousedown', onMouseDown, true);
		canvas.removeEventListener('pointermove', onPointerMove, true);
		canvas.removeEventListener('pointercancel', cancelRoll);
		canvas.removeEventListener('lostpointercapture', onLostPointerCapture);
		window.removeEventListener('mouseup', onMouseUp);
		window.removeEventListener('blur', cancelRoll);
		dispose();
	};
}

function createScene() {
	const cfg = getConfig();
	const scene = new THREE.Scene();
	scene.background = new THREE.Color(THEME.bg);

	// Keep lights available when switching from unlit arrows to a solid volume.
	// Basic materials ignore them, including the low-quality arrow renderer.

	const brightnessValue = get(brightness);

	const dirLight = new THREE.DirectionalLight(0xffffff, 1.8 * brightnessValue);
	dirLight.position.set(1, 2, 3);
	dirLight.userData.baseIntensity = 1.8;
	scene.add(dirLight);

	const fillLight = new THREE.DirectionalLight(0xccccff, 0.8 * brightnessValue);
	fillLight.position.set(-2, 0, 1);
	fillLight.userData.baseIntensity = 0.8;
	scene.add(fillLight);

	const backLight = new THREE.DirectionalLight(0xffffff, 0.5 * brightnessValue);
	backLight.position.set(0, -1, -2);
	backLight.userData.baseIntensity = 0.5;
	scene.add(backLight);

	const ambient = new THREE.AmbientLight(0x8888aa, 1.0 * brightnessValue);
	ambient.userData.baseIntensity = 1.0;
	scene.add(ambient);

	if (cfg.useHemisphere) {
		const hemiLight = new THREE.HemisphereLight(0x8898bf, 0x293245, 0.6 * brightnessValue);
		hemiLight.userData.baseIntensity = 0.6;
		scene.add(hemiLight);
	}

	return scene;
}

function updateSceneLights() {
	const display = get(threeDPreview);
	if (!display) {
		return;
	}

	const brightnessValue = get(brightness);
	display.scene.traverse((child) => {
		if (
			child instanceof THREE.DirectionalLight ||
			child instanceof THREE.AmbientLight ||
			child instanceof THREE.HemisphereLight
		) {
			(child as THREE.Light).intensity = (child.userData.baseIntensity || 1) * brightnessValue;
		}
	});
}

function updateMaterialAppearance() {
	const display = get(threeDPreview);
	if (!display || Array.isArray(display.mesh.material) || display.rendererMode !== 'voxel') {
		return;
	}

	for (const mesh of display.meshes) {
		mesh.material.opacity = get(voxelOpaque) ? 1 : get(voxelOpacity);
		const transparent = !get(voxelOpaque);
		if (mesh.material.transparent !== transparent) {
			mesh.material.transparent = transparent;
			mesh.material.needsUpdate = true;
		}
		mesh.material.depthWrite = get(voxelOpaque);
	}
	requestPreview3DRender();
}

function disposeMesh(mesh: VectorMesh) {
	mesh.dispose();
}

function replacePreviewMesh(_requiredCapacity: number) {
	const display = get(threeDPreview);
	if (!display) return;
	display.topologyKey = '';
	display.lastValues = null;
}

function renderPreviewFrame() {
	renderFrameId = null;
	const display = get(threeDPreview);
	if (!display) {
		return;
	}

	display.controls.update();
	const start = performance.now();
	display.renderer.render(display.scene, display.camera);
	previewPerformance.update((p) => ({
		...p,
		renderMs: performance.now() - start,
		drawCalls: display.renderer.info.render.calls,
		triangles: display.renderer.info.render.triangles,
		lod: get(renderMode) === 'glyph' && glyphSegments() < getConfig().segments
	}));
	cameraRevision.update((revision) => revision + 1);

	if (controlsActive) {
		requestPreview3DRender();
	}
}

export function requestPreview3DRender() {
	if (renderFrameId !== null || !get(threeDPreview)) {
		return;
	}
	renderFrameId = requestAnimationFrame(renderPreviewFrame);
}

function isSampledPosition(x: number, y: number, z: number, step: number, allLayers: boolean) {
	if (step === 1) {
		return true;
	}

	if (x % step !== 0 || y % step !== 0) {
		return false;
	}

	if (allLayers && z % step !== 0) {
		return false;
	}

	return true;
}

let lastVolumeQuantity: string | undefined;
function updateInstances(display: ThreeDPreview) {
	const state = get(previewState),
		layout = getLayout(),
		mode = get(renderMode);
	if (
		mode === 'volume' &&
		state.quantity !== lastVolumeQuantity &&
		(lastVolumeQuantity !== undefined || (state.quantity !== 'm' && state.quantity !== 'geom'))
	) {
		const previous = get(volumeColorMode);
		volumeColorMode.set(
			state.nComp === 1 ? 'value' : previous === 'geometry' || previous === 'value' ? 'x' : previous
		);
		volumeScaleMode.set('data');
	}
	lastVolumeQuantity = state.quantity;
	if ((mode === 'volume' || state.regionActive) && get(previewClientBudget) !== 1000000)
		setPreviewClientBudget(1000000);
	const sourceDepth = getRenderMesh().Nz;
	const positions = state.vectorFieldPositions;
	let values = state.vectorFieldValues;
	if (
		mode === 'volume' &&
		get(volumeColorMode) !== 'geometry' &&
		get(volumeProjection) === 'average'
	) {
		const dimensions: [number, number, number] = [
			getPreviewWidthCells(),
			getPreviewHeightCells(),
			state.allLayers
				? Math.max(
						state.appliedZChosenSize ||
							Math.ceil(sourceDepth / Math.max(state.appliedLayerStride, 1)),
						1
					)
				: 1
		];
		const key = `${get(volumeProjectionAxis)}/${dimensions.join('/')}/${state.appliedLayerStride}/${sourceDepth}`;
		if (projectedSource !== values || projectedPositions !== positions || projectedKey !== key) {
			projectedValues = projectVolumeVectors(
				positions,
				values,
				dimensions,
				Math.max(state.appliedLayerStride || 1, 1),
				get(volumeProjectionAxis),
				state.allLayers ? sourceDepth : 1
			);
			projectedSource = values;
			projectedPositions = positions;
			projectedKey = key;
		}
		values = projectedValues;
	}
	const count = Math.min(
		state.vectorCount,
		Math.floor(values.length / 3),
		Math.floor(positions.length / 3),
		1000000
	);
	const step =
		(mode === 'volume' || state.regionActive
			? 1
			: mode === 'voxel'
				? get(voxelSampling)
				: get(glyphSampling)) * (state.transportSampling || 1);
	const stride = Math.max(state.appliedLayerStride || 1, 1);
	const width = getPreviewWidthCells(),
		height = getPreviewHeightCells();
	const depth = Math.max(state.appliedZChosenSize || Math.ceil(sourceDepth / stride), 1);
	const topologyKey = [
		mode,
		mode === 'volume' ? `${get(volumeColorMode) === 'geometry'}/${get(volumeLighting)}` : '',
		get(qualityLevel),
		state.topologyRevision,
		count,
		mode === 'glyph' ? glyphSegments() : 0,
		step,
		state.allLayers,
		stride,
		layout.stepX,
		layout.stepY,
		layout.stepZ
	].join(':');
	const topologyChanged = display.topologyKey !== topologyKey;
	if (topologyChanged) {
		display.meshes.forEach((mesh) => {
			display.scene.remove(mesh);
			disposeMesh(mesh);
		});
		const bins = count > 131072 ? 4 : 1;
		const groups: number[][] = Array.from(
			{ length: bins * bins * (state.allLayers ? bins : 1) },
			() => []
		);
		for (let i = 0; i < count; i++) {
			const o = i * 3,
				x = positions[o],
				y = positions[o + 1],
				z = Math.floor(positions[o + 2] / stride);
			if (!isSampledPosition(x, y, z, step, state.allLayers)) continue;
			const bx = Math.min(bins - 1, Math.floor((x * bins) / width));
			const by = Math.min(bins - 1, Math.floor((y * bins) / height));
			const bz = state.allLayers ? Math.min(bins - 1, Math.floor((z * bins) / depth)) : 0;
			if (bx < 0 || by < 0 || bz < 0) continue;
			groups[(bz * bins + by) * bins + bx].push(i);
		}
		display.blocks = groups.filter((g) => g.length).map((g) => Uint32Array.from(g));
		if (!display.blocks.length) display.blocks = [new Uint32Array()];
		display.meshes = display.blocks.map((indices) => createMesh(indices.length));
		display.liveIndices = display.blocks.map((indices) =>
			new Uint32Array(indices.length).fill(0xffffffff)
		);
		display.mesh = display.meshes[0];
		display.meshCapacity = display.meshes.reduce((n, m) => n + m.capacity, 0);
		display.meshes.forEach((mesh) => display.scene.add(mesh));
		display.topologyKey = topologyKey;
		display.rendererMode = mode;
	}
	const colorMode = mode === 'volume' ? get(volumeColorMode) : get(voxelColorMode),
		threshold = get(voxelThreshold);
	const filterKey = [mode, colorMode, threshold].join(':');
	const physicalScale = state.normScale || 1;
	let min = Infinity,
		max = -Infinity;
	if (colorMode !== 'geometry' && colorMode !== 'orientation') {
		for (let i = 0; i < count; i++) {
			if (state.vectorOccupancy?.[i] === 0) continue;
			const value =
				componentValue(values[i * 3], values[i * 3 + 1], values[i * 3 + 2], colorMode) *
				physicalScale;
			if (Number.isFinite(value)) {
				min = Math.min(min, value);
				max = Math.max(max, value);
			}
		}
	}
	if (!Number.isFinite(min)) {
		min = 0;
		max = 1;
	}
	if (get(volumeScaleMode) === 'manual') [min, max] = get(volumeManualRange);
	const fieldScale = getColorScale(min, max);
	if (get(volumeScaleMode) === 'manual') {
		fieldScale.min = min;
		fieldScale.max = max;
	}
	volumeLegend.set(fieldScale);
	let visible = 0,
		uploaded = 0;
	const rebuild =
		topologyChanged ||
		display.lastValues !== values ||
		display.lastOccupancy !== state.vectorOccupancy ||
		display.filterKey !== filterKey;
	for (let block = 0; block < display.meshes.length; block++) {
		const mesh = display.meshes[block];
		const scale = mode === 'volume' ? 1 : Math.max(0.12, step * (1 - get(voxelGap)));
		mesh.uniforms.previewFieldScale.value =
			colorMode !== 'orientation' && colorMode !== 'geometry' ? 1 : 0;
		mesh.uniforms.previewRange.value.set(fieldScale.min, fieldScale.max);
		mesh.uniforms.previewNorm.value = physicalScale;
		mesh.uniforms.previewPaletteSize.value = fieldScale.palette.length;
		for (let i = 0; i < 7; i++)
			mesh.uniforms.previewColors.value[i]
				.set(fieldScale.palette[Math.min(i, fieldScale.palette.length - 1)])
				.convertLinearToSRGB();
		mesh.uniforms.previewSolid.value = mode === 'volume' && colorMode === 'geometry' ? 1 : 0;
		mesh.uniforms.previewMode.value = mode === 'volume' ? 2 : mode === 'voxel' ? 1 : 0;
		mesh.uniforms.previewScale.value.set(
			mode !== 'glyph' ? scale * layout.stepX : layout.glyphScale,
			mode !== 'glyph'
				? scale * (state.allLayers ? layout.stepZ : layout.zCell)
				: layout.glyphScale,
			mode !== 'glyph' ? scale * layout.stepY : layout.glyphScale
		);
		mesh.uniforms.previewColorMode.value =
			colorMode === 'magnitude'
				? 4
				: colorMode === 'value'
					? 1
					: Math.max(0, ['orientation', 'x', 'y', 'z'].indexOf(colorMode));
		mesh.uniforms.previewTopo.value = mode === 'voxel' && get(topoEnabled) ? 1 : 0;
		mesh.uniforms.previewTopoAxis.value = ['x', 'y', 'z'].indexOf(get(topoComponent));
		mesh.uniforms.previewTopoScale.value = get(topoMultiplier) * layout.glyphScale;
		const padding =
			Math.max(...mesh.uniforms.previewScale.value.toArray()) +
			(mode === 'voxel' && get(topoEnabled)
				? mesh.maxVectorComponent * Math.abs(mesh.uniforms.previewTopoScale.value)
				: 0);
		if (!rebuild) {
			mesh.updateBounds(padding);
			visible += mesh.count;
			continue;
		}
		mesh.maxVectorComponent = 0;
		const offsets = mesh.offsets.array as Float32Array,
			fields = mesh.vectors.array as Float32Array;
		const previous = display.liveIndices[block];
		let n = 0,
			positionsChanged = topologyChanged;
		let minX = Infinity,
			minY = Infinity,
			minZ = Infinity,
			maxX = -Infinity,
			maxY = -Infinity,
			maxZ = -Infinity;
		for (const i of display.blocks[block]) {
			if (mode === 'volume' && state.vectorOccupancy?.[i] === 0) continue;
			const o = i * 3,
				vx = values[o],
				vy = values[o + 1],
				vz = values[o + 2];
			if (!Number.isFinite(vx) || !Number.isFinite(vy) || !Number.isFinite(vz)) continue;
			if (mode === 'glyph' && vx === 0 && vy === 0 && vz === 0) continue;
			if (mode === 'voxel') {
				// Changing color must not hide cells with a zero selected component.
				if (vx * vx + vy * vy + vz * vz < threshold * threshold) continue;
			}
			const target = n * 3;
			fields[target] = vx;
			fields[target + 1] = vy;
			fields[target + 2] = vz;
			mesh.maxVectorComponent = Math.max(
				mesh.maxVectorComponent,
				Math.abs(vx),
				Math.abs(vy),
				Math.abs(vz)
			);
			if (previous[n] !== i) {
				positionsChanged = true;
				previous[n] = i;
			}
			const x = (positions[o] + 0.5) * layout.stepX;
			const binZ = Math.floor(positions[o + 2] / stride),
				binDepth = state.allLayers ? Math.min(stride, Math.max(sourceDepth - binZ * stride, 1)) : 1;
			mesh.depths.array[n] = mode === 'volume' && state.allLayers ? binDepth / stride : 1;
			const y =
				mode === 'volume' && state.allLayers
					? (binZ * stride + binDepth / 2) * layout.zCell
					: state.allLayers
						? (Math.floor(positions[o + 2] / stride) + 0.5) * layout.stepZ
						: layout.zCell / 2;
			const z = (positions[o + 1] + 0.5) * layout.stepY;
			offsets[target] = x;
			offsets[target + 1] = y;
			offsets[target + 2] = z;
			minX = Math.min(minX, x);
			maxX = Math.max(maxX, x);
			minY = Math.min(minY, y);
			maxY = Math.max(maxY, y);
			minZ = Math.min(minZ, z);
			maxZ = Math.max(maxZ, z);
			n++;
		}
		if (n !== mesh.count) positionsChanged = true;
		mesh.count = n;
		mesh.upload(positionsChanged);
		// Conservative bounds cover the full allowed topography range and glyph head.
		if (n)
			mesh.setBounds(
				new THREE.Vector3((minX + maxX) / 2, (minY + maxY) / 2, (minZ + maxZ) / 2),
				Math.hypot(maxX - minX, maxY - minY, maxZ - minZ) / 2
			);
		mesh.updateBounds(
			Math.max(...mesh.uniforms.previewScale.value.toArray()) +
				(mode === 'voxel' && get(topoEnabled)
					? mesh.maxVectorComponent * Math.abs(mesh.uniforms.previewTopoScale.value)
					: 0)
		);
		visible += n;
		uploaded += n * (positionsChanged ? 28 : 16);
	}
	display.lastValues = values;
	display.lastOccupancy = state.vectorOccupancy;
	display.filterKey = filterKey;
	visibleRenderCount.set(visible);
	previewPerformance.update((p) => ({ ...p, uploadBytes: uploaded }));
	updateMaterialAppearance();
}

function init() {
	const scene = createScene();
	const camera = createCamera();
	const renderer = createRenderer();
	const controls = createControls(camera, renderer);
	const mesh = createMesh();

	scene.add(mesh);

	threeDPreview.set({
		mesh,
		meshes: [mesh],
		blocks: [],
		liveIndices: [],
		topologyKey: '',
		lastValues: null,
		filterKey: '',
		meshCapacity: nextMeshCapacity(Math.max(1)),
		scene,
		camera,
		renderer,
		controls,
		rendererMode: get(renderMode),
		shapeKey: getPreviewShapeKey()
	});
	controls.addEventListener('change', () => {
		scheduleInstanceUpdate();
		requestPreview3DRender();
	});
	controls.addEventListener('start', () => {
		controlsActive = true;
		renderer.setPixelRatio(Math.min(getConfig().pixelRatio, 1));
		requestPreview3DRender();
	});
	controls.addEventListener('end', () => {
		controlsActive = false;
		renderer.setPixelRatio(getConfig().pixelRatio);
		requestPreview3DRender();
	});

	update();

	const container = document.getElementById('container');
	if (container) {
		if (!resizeObserver) {
			resizeObserver = new ResizeObserver(() => {
				const current = document.getElementById('container');
				if (!current) {
					return;
				}

				const width = current.clientWidth;
				const height = current.clientHeight;
				renderer.setSize(width, height);
				camera.aspect = width / height;
				camera.updateProjectionMatrix();
				requestPreview3DRender();
			});
		}

		resizeObserver.disconnect();
		resizeObserver.observe(container);
	}

	requestPreview3DRender();
}

function update() {
	const display = get(threeDPreview);
	if (!display) return;
	const start = performance.now();
	updateInstances(display);
	previewPerformance.update((p) => ({ ...p, updateMs: performance.now() - start }));
	requestPreview3DRender();
}

let updateFrameId: number | null = null;
function scheduleInstanceUpdate() {
	if (updateFrameId !== null) return;
	updateFrameId = requestAnimationFrame(() => {
		updateFrameId = null;
		update();
	});
}

export function preview3D() {
	if (!has3DPreviewData()) {
		disposePreview3D();
		return;
	}

	const display = get(threeDPreview);
	if (!display) {
		disposePreview2D();
		init();
		return;
	}

	if (getPreviewShapeKey() !== display.shapeKey || get(previewState).refresh) {
		disposePreview2D();
		const shapeKey = getPreviewShapeKey();
		if (shapeKey !== display.shapeKey) {
			display.shapeKey = shapeKey;
			if (get(previewState).regionActive) fitPreviewRegion();
			else resetCamera();
		}
	}

	update();
}

export function disposePreview3D() {
	projectedSource = null;
	projectedPositions = null;
	projectedValues = new Float32Array();
	projectedKey = '';
	const container = document.getElementById('container');
	const display = get(threeDPreview);
	visibleRenderCount.set(0);

	controlsActive = false;
	if (updateFrameId !== null) {
		cancelAnimationFrame(updateFrameId);
		updateFrameId = null;
	}
	if (renderFrameId !== null) {
		cancelAnimationFrame(renderFrameId);
		renderFrameId = null;
	}

	if (display) {
		display.controls.dispose();
		display.renderer.dispose();
		display.scene.traverse((child) => {
			if (child instanceof THREE.Mesh) {
				child.geometry.dispose();
				if (Array.isArray(child.material)) {
					child.material.forEach((material) => material.dispose());
				} else if (child.material) {
					child.material.dispose();
				}
			}
		});
		threeDPreview.set(null);

		if (container) {
			container.innerHTML = '';
		}
	}

	if (resizeObserver) {
		resizeObserver.disconnect();
		resizeObserver = null;
	}
}

export function resizeECharts() {
	const container = document.getElementById('container');
	if (!container) {
		return;
	}

	if (!resizeObserver) {
		resizeObserver = new ResizeObserver(() => {
			const display = get(threeDPreview);
			if (!display) {
				return;
			}

			display.renderer.setSize(container.clientWidth, container.clientHeight);
			display.camera.aspect = container.clientWidth / container.clientHeight;
			display.camera.updateProjectionMatrix();
		});
	}

	resizeObserver.disconnect();
	resizeObserver.observe(container);

	const display = get(threeDPreview);
	if (display) {
		display.renderer.setSize(container.clientWidth, container.clientHeight);
		display.camera.aspect = container.clientWidth / container.clientHeight;
		display.camera.updateProjectionMatrix();
		requestPreview3DRender();
	}
}

export function setBrightness(value: number) {
	const clamped = Math.max(0.3, Math.min(3.0, value));
	persistSetting(STORAGE_KEYS.brightness, clamped);
	brightness.set(clamped);
	updateSceneLights();
	requestPreview3DRender();
}

export function setQuality(level: QualityLevel) {
	persistSetting(STORAGE_KEYS.quality, level);
	qualityLevel.set(level);
	rebuildPreview3D();
}

export function setRenderMode(mode: Preview3DRenderMode) {
	persistSetting(STORAGE_KEYS.renderMode, mode);
	renderMode.set(mode);
	if (mode === 'volume') {
		setPreviewClientBudget(1000000);
		if (!get(previewState).allLayers && get(meshState).Nz > 1) postAllLayers(true);
	}
	const count = get(previewState).vectorCount;
	if (get(threeDPreview)) {
		replacePreviewMesh(count);
		update();
	}
}

export function setVolumeProjection(
	mode: 'surface' | 'average',
	axis: ProjectionAxis = get(volumeProjectionAxis)
) {
	volumeProjection.set(mode);
	volumeProjectionAxis.set(axis);
	if (mode === 'average' && !get(previewState).allLayers) postAllLayers(true);
	scheduleInstanceUpdate();
}
export function setVolumeLighting(enabled: boolean) {
	volumeLighting.set(enabled);
	scheduleInstanceUpdate();
}
export function setVolumeScale(
	mode: 'data' | 'manual',
	min = get(volumeManualRange)[0],
	max = get(volumeManualRange)[1]
) {
	if (!Number.isFinite(min) || !Number.isFinite(max) || min >= max) return;
	volumeScaleMode.set(mode);
	volumeManualRange.set([min, max]);
	scheduleInstanceUpdate();
}
export function setVolumeColorMode(mode: VolumeColorMode) {
	volumeColorMode.set(mode);
	scheduleInstanceUpdate();
}

export function setVoxelOpaque(value: boolean) {
	voxelOpaque.set(value);
	persistSetting('preview3d_opaque', String(value));
	updateMaterialAppearance();
}
export function setGlyphSampling(value: VoxelSampling) {
	glyphSampling.set(value);
	scheduleInstanceUpdate();
}
export function setVoxelOpacity(value: number) {
	const clamped = Math.max(0.15, Math.min(0.95, value));
	persistSetting(STORAGE_KEYS.voxelOpacity, clamped);
	voxelOpacity.set(clamped);
	updateMaterialAppearance();
}

export function setVoxelGap(value: number) {
	const clamped = Math.max(0.02, Math.min(0.42, value));
	persistSetting(STORAGE_KEYS.voxelGap, clamped);
	voxelGap.set(clamped);
	if (get(renderMode) === 'voxel') {
		scheduleInstanceUpdate();
	}
}

export function setVoxelThreshold(value: number) {
	const clamped = Math.max(0, Math.min(0.95, value));
	persistSetting(STORAGE_KEYS.voxelThreshold, clamped);
	voxelThreshold.set(clamped);
	if (get(renderMode) === 'voxel') {
		scheduleInstanceUpdate();
	}
}

export function setVoxelColorMode(mode: VoxelColorMode) {
	persistSetting(STORAGE_KEYS.voxelColorMode, mode);
	voxelColorMode.set(mode);
	if (get(renderMode) !== 'volume') {
		scheduleInstanceUpdate();
	}
}

export function setVoxelSampling(value: VoxelSampling) {
	persistSetting(STORAGE_KEYS.voxelSampling, value);
	voxelSampling.set(value);
	if (get(renderMode) === 'voxel') {
		scheduleInstanceUpdate();
	}
}

export function setTopoEnabled(value: boolean) {
	persistSetting(STORAGE_KEYS.topoEnabled, String(value));
	topoEnabled.set(value);
	if (get(renderMode) === 'voxel') {
		scheduleInstanceUpdate();
	}
}

export function setTopoComponent(comp: TopoComponent) {
	persistSetting(STORAGE_KEYS.topoComponent, comp);
	topoComponent.set(comp);
	if (get(renderMode) === 'voxel' && get(topoEnabled)) {
		scheduleInstanceUpdate();
	}
}

export function setTopoMultiplier(value: number) {
	const clamped = Math.max(0.5, Math.min(50, value));
	persistSetting(STORAGE_KEYS.topoMultiplier, clamped);
	topoMultiplier.set(clamped);
	if (get(renderMode) === 'voxel' && get(topoEnabled)) {
		scheduleInstanceUpdate();
	}
}

export function fitPreviewRegion() {
	const display = get(threeDPreview);
	if (!display) return;
	const { centerX, centerY, centerZ, xSize, ySize, depthCells } = getWorldExtents();
	const vertical = THREE.MathUtils.degToRad(display.camera.fov) / 2;
	const horizontal = Math.atan(Math.tan(vertical) * display.camera.aspect);
	const orbitDistance =
		(Math.hypot(xSize, ySize, depthCells) / 2 / Math.sin(Math.min(vertical, horizontal))) * 1.1;
	const direction = display.camera.position.clone().sub(display.controls.target).normalize();
	display.controls.target.set(centerX, centerY, centerZ);
	display.camera.position.copy(display.controls.target).addScaledVector(direction, orbitDistance);
	display.controls.update();
	requestPreview3DRender();
}

export function resetCamera() {
	const display = get(threeDPreview);

	if (!display) {
		return;
	}

	const { centerX, centerY, centerZ } = getWorldExtents();
	setDefaultCameraPose(display.camera);
	display.controls.target.set(centerX, centerY, centerZ);
	display.controls.update();
	requestPreview3DRender();
}

export function setCameraViewDirection(dx: number, dy: number, dz: number) {
	const display = get(threeDPreview);
	if (!display) return;

	const len = Math.sqrt(dx * dx + dy * dy + dz * dz);
	if (len === 0) return;

	const worldDir = setWorldDirectionFromSimulation(_tempVec, {
		x: dx / len,
		y: dy / len,
		z: dz / len
	}).normalize();
	const { centerX, centerY, centerZ, orbitDistance } = getWorldExtents();

	let ux = 0,
		uy = 1,
		uz = 0;
	if (Math.abs(worldDir.y) > 0.9) {
		ux = 0;
		uy = 0;
		uz = worldDir.y > 0 ? 1 : -1;
	}

	display.camera.position.set(
		centerX + worldDir.x * orbitDistance,
		centerY + worldDir.y * orbitDistance,
		centerZ + worldDir.z * orbitDistance
	);
	display.camera.up.set(ux, uy, uz);
	display.camera.lookAt(centerX, centerY, centerZ);
	display.controls.target.set(centerX, centerY, centerZ);
	display.controls.update();
	requestPreview3DRender();
}

export function orbitCamera(deltaX: number, deltaY: number) {
	const display = get(threeDPreview);
	if (!display) return;

	const camera = display.camera;
	const target = display.controls.target;
	const offset = new THREE.Vector3().subVectors(camera.position, target);
	const radius = offset.length();

	let theta = Math.atan2(offset.x, offset.z);
	let phi = Math.asin(THREE.MathUtils.clamp(offset.y / radius, -1, 1));

	theta -= deltaX * 0.01;
	phi += deltaY * 0.01;
	phi = THREE.MathUtils.clamp(phi, -Math.PI / 2 + 0.05, Math.PI / 2 - 0.05);

	offset.x = radius * Math.cos(phi) * Math.sin(theta);
	offset.y = radius * Math.sin(phi);
	offset.z = radius * Math.cos(phi) * Math.cos(theta);

	camera.position.copy(target).add(offset);
	camera.up.set(0, 1, 0);
	camera.lookAt(target);
	display.controls.update();
	requestPreview3DRender();
}

export function getCameraMatrix(): string {
	const display = get(threeDPreview);
	if (!display) return 'none';

	const cam = display.camera;
	const target = display.controls.target;

	// Direction from target toward camera (NOT camera toward target)
	const dir = _camDir.subVectors(cam.position, target).normalize();

	// Spherical angles: theta = horizontal (around Y), phi = vertical
	const theta = Math.atan2(dir.x, dir.z); // 0 when camera at +Z (front)
	const phi = Math.asin(THREE.MathUtils.clamp(dir.y, -1, 1));

	// CSS rotations: rotate the cube so the face the camera sees is toward the viewer
	// rotateY(-theta): horizontal orbit
	// rotateX(phi): vertical tilt (positive phi = camera above = tilt cube forward)
	return `rotateX(${phi}rad) rotateY(${-theta}rad)`;
}
