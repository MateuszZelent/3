import {test,expect} from '@playwright/test';
import {encode} from '@msgpack/msgpack';
test('surface quantity and visible progressive view transitions',async({page})=>{
 const nodeBufferModule:string='node:buffer';const {Buffer}=await import(nodeBufferModule);
 let stream:any,main:any,sequence=1,component='3D',quantity='m';
 const errors:string[]=[];page.on('pageerror',e=>errors.push(e.message));
 function frame(type='3D'){
 const scalar=quantity==='alpha',three=type==='3D';
 stream.send(Buffer.from(encode({sequence:sequence++,timestamp:Date.now(),quantity,component,nComp:scalar?1:3,type,allLayers:true,unit:'',normScale:scalar?.02:1,vectorCount:three?8:0,topologyRevision:sequence,appliedXChosenSize:three?2:512,appliedYChosenSize:three?2:512,appliedZChosenSize:2,appliedLayerStride:1,xChosenSize:2,yChosenSize:2,zChosenSize:2,xPossibleSizes:[1,2],yPossibleSizes:[1,2],zPossibleSizes:[1,2],maxPoints:1000000,dataPointsCount:three?8:262144,autoScaleEnabled:false,autoDownscaled:false,plane:'xy',planeUChosenSize:512,planeVChosenSize:512,appliedPlaneUSize:512,appliedPlaneVSize:512,planeUPossibleSizes:[512],planeVPossibleSizes:[512],min:.02,max:.02,scalarField:three?[]:Array.from({length:262144},(_,i)=>[i%512,Math.floor(i/512),.02]),vectorValuesBinary:new Uint8Array(new Float32Array(Array.from({length:24},(_,i)=>i%3===0?1:0)).buffer),vectorPositionsBinary:new Uint8Array(new Int32Array(Array.from({length:24},(_,i)=>i%3===0?Math.floor(i/3)%2:i%3===1?Math.floor(i/6)%2:Math.floor(i/12))).buffer)})));
 }
 await page.routeWebSocket(/\/ws\/preview$/,s=>{stream=s});await page.routeWebSocket(/\/ws$/,s=>{main=s});
 await page.route('**/api/preview/quantity',async r=>{await new Promise(resolve=>setTimeout(resolve,450));quantity=r.request().postDataJSON().quantity;frame();await r.fulfill({json:null});});
 await page.route('**/api/preview/component',async r=>{await new Promise(resolve=>setTimeout(resolve,450));component=r.request().postDataJSON().component;frame(component==='3D'?'3D':'2D');await r.fulfill({json:null});});
 await page.route('**/api/preview/allLayers',r=>r.fulfill({json:null}));
 await page.goto('/');await expect.poll(()=>!!stream&&!!main).toBeTruthy();
 main.send(Buffer.from(encode({mesh:{Nx:512,Ny:512,Nz:2,dx:1e-9,dy:1e-9,dz:1e-9}})));frame();
 await page.getByRole('button',{name:'Volume',exact:true}).click();await expect(page.getByLabel('Color by',{exact:true})).toHaveValue('geometry');
 await page.getByLabel('Quantity',{exact:true}).selectOption('alpha');
 await expect(page.getByRole('status').filter({hasText:'Loading alpha'})).toBeVisible();
 await expect(page.locator('#container')).toHaveAttribute('aria-busy','false');
 await expect(page.getByLabel('Color by',{exact:true})).toHaveValue('value');
 await page.evaluate(()=>{const w=window as any;w.renderGaps=[];let previous=performance.now();w.renderHeartbeat=setInterval(()=>{const now=performance.now();w.renderGaps.push(now-previous);previous=now},16);});
 const start=Date.now();await page.getByRole('button',{name:'2D section',exact:true}).click();
 await expect(page.getByRole('status').filter({hasText:'Switching to 2D section'})).toBeVisible();
 await expect(page.getByRole('status').filter({hasText:'Rendering the new view'})).toBeVisible();
 await expect(page.locator('#container')).toHaveAttribute('aria-busy','false',{timeout:20000});
 const switchMs=Date.now()-start;expect(switchMs).toBeLessThan(20000);
 const maxUiGap=await page.evaluate(()=>{const w=window as any;clearInterval(w.renderHeartbeat);return Math.max(...w.renderGaps)});expect(maxUiGap).toBeLessThan(1000);
 await page.getByRole('button',{name:'3D volume',exact:true}).click();
 await expect(page.getByRole('status').filter({hasText:'Switching to 3D volume'})).toBeVisible();
 await expect(page.locator('#container')).toHaveAttribute('aria-busy','false');expect(errors).toEqual([]);
 console.log(JSON.stringify({largePlaneCells:262144,switchMs,maxUiGap}));
});
