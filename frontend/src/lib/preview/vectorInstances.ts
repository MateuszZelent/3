import * as THREE from 'three';

// Compact instancing: the GPU owns orientation, topography and the palette.
// No mat4 or instance color is stored/uploaded for each vector.
export class VectorMesh extends THREE.Mesh<THREE.InstancedBufferGeometry, THREE.Material> {
	maxVectorComponent = 1;
	baseBounds = new THREE.Sphere();
	offsets: THREE.InstancedBufferAttribute;
	vectors: THREE.InstancedBufferAttribute;
	depths: THREE.InstancedBufferAttribute;
	uniforms = {
		previewMode: { value: 0 },
		previewSolid: { value: 0 },
		previewFieldScale: { value: 0 },
		previewNorm: { value: 1 },
		previewRange: { value: new THREE.Vector2(0, 1) },
		previewColors: { value: Array.from({ length: 7 }, () => new THREE.Color()) },
		previewPaletteSize: { value: 7 },
		previewScale: { value: new THREE.Vector3(1, 1, 1) },
		previewColorMode: { value: 0 },
		previewTopo: { value: 0 },
		previewTopoAxis: { value: 2 },
		previewTopoScale: { value: 1 },
		previewNegative: { value: new THREE.Color('#2f6caa') },
		previewNeutral: { value: new THREE.Color('#f4f1ed') },
		previewPositive: { value: new THREE.Color('#cf6256') }
	};
	constructor(
		base: THREE.BufferGeometry,
		material: THREE.Material,
		public capacity: number
	) {
		const geometry = new THREE.InstancedBufferGeometry();
		geometry.index = base.index;
		for (const [name, attribute] of Object.entries(base.attributes))
			geometry.setAttribute(name, attribute);
		geometry.setAttribute(
			'color',
			new THREE.BufferAttribute(new Float32Array(base.attributes.position.count * 3).fill(1), 3)
		);
		super(geometry, material);
		this.offsets = new THREE.InstancedBufferAttribute(new Float32Array(capacity * 3), 3).setUsage(
			THREE.DynamicDrawUsage
		);
		this.vectors = new THREE.InstancedBufferAttribute(new Float32Array(capacity * 3), 3).setUsage(
			THREE.DynamicDrawUsage
		);
		geometry.setAttribute('previewOffset', this.offsets);
		geometry.setAttribute('previewVector', this.vectors);
		this.depths = new THREE.InstancedBufferAttribute(
			new Float32Array(capacity).fill(1),
			1
		).setUsage(THREE.DynamicDrawUsage);
		geometry.setAttribute('previewDepth', this.depths);
		geometry.instanceCount = 0;
		(material as THREE.MeshBasicMaterial).vertexColors = true;
		material.onBeforeCompile = (shader) => {
			Object.assign(shader.uniforms, this.uniforms);
			shader.vertexShader = shader.vertexShader
				.replace('#include <common>', `#include <common>\n${vectorGLSL}`)
				.replace(
					'#include <beginnormal_vertex>',
					`#include <beginnormal_vertex>\nobjectNormal = previewNormal(objectNormal);`
				)
				.replace('#include <begin_vertex>', `vec3 transformed = previewTransform(position);`)
				.replace('#include <color_vertex>', `#include <color_vertex>\nvColor = previewPalette();`);
		};
		material.customProgramCacheKey = () => 'preview-vector-v2';
	}
	get count() {
		return this.geometry.instanceCount;
	}
	set count(value: number) {
		this.geometry.instanceCount = value;
	}
	setBounds(center: THREE.Vector3, radius: number) {
		this.baseBounds.set(center, radius);
		this.geometry.boundingSphere = this.baseBounds.clone();
	}
	updateBounds(padding: number) {
		this.geometry.boundingSphere = this.baseBounds.clone();
		this.geometry.boundingSphere.radius += padding;
	}
	upload(positionsChanged: boolean) {
		for (const attribute of positionsChanged
			? [this.offsets, this.vectors, this.depths]
			: [this.vectors, this.depths]) {
			attribute.clearUpdateRanges();
			if (this.count > 0) {
				attribute.addUpdateRange(0, this.count * attribute.itemSize);
				attribute.needsUpdate = true;
			}
		}
	}
	dispose() {
		this.geometry.dispose();
		this.material.dispose();
	}
}

export const vectorGLSL = `
attribute vec3 previewOffset;
attribute vec3 previewVector;
attribute float previewDepth;
uniform float previewMode;
uniform float previewSolid;
uniform float previewFieldScale;
uniform float previewNorm;
uniform vec2 previewRange;
uniform vec3 previewColors[7];
uniform int previewPaletteSize;
uniform vec3 previewScale;
uniform float previewColorMode;
uniform float previewTopo;
uniform int previewTopoAxis;
uniform float previewTopoScale;
uniform vec3 previewNegative;
uniform vec3 previewNeutral;
uniform vec3 previewPositive;
vec4 previewRotation() {
 vec3 direction=previewVector.xzy;
 float magnitude=max(max(abs(direction.x),abs(direction.y)),abs(direction.z));
 vec3 d=normalize(direction/max(magnitude,1.0e-30));
 // Quaternion rotating +Y onto d; explicitly handle the antiparallel case.
 if (d.y < -0.999999) return vec4(0.0,0.0,1.0,0.0);
 return normalize(vec4(d.z,0.0,-d.x,1.0+d.y));
}
vec3 previewRotate(vec3 p) {
 vec4 q=previewRotation();
 return p + 2.0*cross(q.xyz,cross(q.xyz,p)+q.w*p);
}
float previewDisplacement() {
 float value=clamp(previewVector[previewTopoAxis]*previewTopo*previewTopoScale,-1.0e20,1.0e20);
 return abs(value)<0.000001 ? 0.0 : value;
}
vec3 previewSize() {
 vec3 size=previewScale;
 if(previewMode>1.5) size.y*=previewDepth;
 if(previewMode>0.5) size.y+=abs(previewDisplacement());
 return max(size,vec3(0.000001));
}
vec3 previewTransform(vec3 p) {
 if(previewMode<0.5) return previewOffset+previewRotate(p*previewScale);
 vec3 offset=previewOffset; offset.y+=previewDisplacement()*0.5;
 return offset+p*previewSize();
}
vec3 previewNormal(vec3 n) {
 if(previewMode<0.5) return previewRotate(n);
 return n/previewSize();
}
vec3 previewSRGB(vec3 c) {
 return mix(c/12.92,pow((c+0.055)/1.055,vec3(2.4)),step(vec3(0.04045),c));
}
vec3 previewPalette() {
 if(previewSolid>0.5) return previewSRGB(vec3(0.55,0.78,0.75));
 if(previewFieldScale>0.5) {
  float value=previewVector[int(previewColorMode)-1]*previewNorm;
  float span=previewRange.y-previewRange.x;
  float t=span>0.0?clamp((value-previewRange.x)/span,0.0,1.0):0.5;
  float p=t*float(previewPaletteSize-1);int i=min(int(floor(p)),previewPaletteSize-2);
  vec3 lo=previewColors[i], hi=previewColors[i+1];
  // ECharts interpolates the shared color stops in sRGB; convert after interpolation.
  return previewSRGB(mix(lo,hi,p-float(i)));
 }
 if(previewColorMode>0.5) {
  int axis=int(previewColorMode)-1;
  float v=clamp(previewVector[axis],-1.0,1.0);
  return mix(previewNeutral,v<0.0?previewNegative:previewPositive,abs(v));
 }
 float sat=min(1.0,length(previewVector));
 float l=clamp(0.5*previewVector.z+0.5,0.0,1.0);
 float h=mod(atan(previewVector.y,previewVector.x)/1.0471975512,6.0);
 float c=(l<=0.5?2.0*l:2.0-2.0*l)*sat;
 float x=c*(1.0-abs(mod(h,2.0)-1.0));
 vec3 rgb;
 if(h<1.0)rgb=vec3(c,x,0.0);
 else if(h<2.0)rgb=vec3(x,c,0.0);
 else if(h<3.0)rgb=vec3(0.0,c,x);
 else if(h<4.0)rgb=vec3(0.0,x,c);
 else if(h<5.0)rgb=vec3(x,0.0,c);
 else rgb=vec3(c,0.0,x);
 return previewSRGB(floor(clamp(rgb+vec3(l-0.5*c),0.0,1.0)*255.0)/255.0);
}
`;
