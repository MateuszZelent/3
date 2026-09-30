const { chromium } = await import(new URL('../../../frontend/node_modules/playwright/index.mjs', import.meta.url));
import { writeFile } from 'node:fs/promises';
const browser = await chromium.launch({headless:true,args:['--enable-unsafe-swiftshader']});
const page = await browser.newPage({viewport:{width:1024,height:768}});
await page.routeWebSocket(/\/ws/,socket=>socket.close());
await page.goto(process.env.AUDIT_URL || 'http://127.0.0.1:4175/');
await page.waitForSelector('#container');
const result = await page.evaluate(async () => {
  const {get} = await import('/node_modules/.vite/deps/svelte_store.js');
  const {meshState} = await import('/src/api/incoming/mesh.ts');
  const {previewState} = await import('/src/api/incoming/preview.ts');
  const mod = await import('/src/lib/preview/preview3D.ts');
  const rows=[];
  mod.qualityLevel.set('high'); mod.voxelSampling.set(1); mod.voxelThreshold.set(.08); mod.voxelColorMode.set('orientation'); mod.topoEnabled.set(false);
  for(const n of [200000,262144,1000000]) {
    mod.disposePreview3D();
    const nx=n===262144?64:100, ny=nx, nz=n/(nx*ny);
    meshState.set({Nx:nx,Ny:ny,Nz:nz,dx:1,dy:1,dz:1});
    const values=new Float32Array(n*3), positions=new Int32Array(n*3);
    for(let i=0;i<n;i++) {
      values[i*3]=Math.sin(i*.01)*.5; values[i*3+1]=.6; values[i*3+2]=.4;
      positions[i*3]=i%nx; positions[i*3+1]=Math.floor(i/nx)%ny; positions[i*3+2]=Math.floor(i/(nx*ny));
    }
    previewState.update(s=>({...s,type:'3D',nComp:3,allLayers:true,appliedXChosenSize:nx,appliedYChosenSize:ny,appliedLayerStride:1,vectorFieldValues:values,vectorFieldPositions:positions,vectorCount:n,refresh:false}));
    // All synchronous calls in this evaluate run before any RAF/GPU draw.
    for(const mode of ['glyph','voxel']) {
      mod.renderMode.set(mode);
      const start=performance.now(); mod.preview3D(); const first=performance.now()-start;
      const samples=[];
      for(let j=0;j<7;j++){previewState.update(s=>({...s,vectorFieldValues:s.vectorFieldValues.slice()}));const t=performance.now();mod.preview3D();samples.push(performance.now()-t);}
      const display=get(mod.threeDPreview); if(!display)throw new Error(JSON.stringify({n,mode,state:{type:get(previewState).type,nComp:get(previewState).nComp}}));
      rows.push({n,mode,firstMs:first,samplesMs:samples,visible:get(mod.visibleRenderCount),capacity:display.meshCapacity,blocks:display.meshes.length,trianglesPerInstance:display.mesh.geometry.index.count/3,offsetBytes:display.meshes.reduce((n,m)=>n+m.offsets.array.byteLength,0),vectorBytes:display.meshes.reduce((n,m)=>n+m.vectors.array.byteLength,0),uploadBytes:get(mod.previewPerformance).uploadBytes});
    }
  }
  for(const step of [2,4]) { mod.voxelSampling.set(step); const samples=[]; for(let j=0;j<7;j++){previewState.update(s=>({...s,vectorFieldValues:s.vectorFieldValues.slice()}));const t=performance.now();mod.preview3D();samples.push(performance.now()-t);} rows.push({n:1000000,mode:'voxel',step,samplesMs:samples,visible:get(mod.visibleRenderCount),capacity:get(mod.threeDPreview).meshCapacity}); }
  const d=get(mod.threeDPreview), gl=d.renderer.getContext(), ext=gl.getExtension('WEBGL_debug_renderer_info');
  const renderer=ext?gl.getParameter(ext.UNMASKED_RENDERER_WEBGL):gl.getParameter(gl.RENDERER);
  mod.disposePreview3D();
  return {rows,renderer,userAgent:navigator.userAgent,notes:'Synchronous CPU update only; no GPU draw, transport or backend measured; 7 warm calls, new vector buffer each warm call; adaptive screen geometry LOD, high material quality, orientation color, threshold 0.08, topography off; fresh browser context.'};
});
await writeFile(process.env.AUDIT_OUTPUT || '/tmp/3-audit-bench-after.json',JSON.stringify(result,null,2));
console.log(JSON.stringify(result));
await browser.close();
