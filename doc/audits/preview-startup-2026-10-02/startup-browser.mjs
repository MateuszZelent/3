import {chromium} from '../../../frontend/node_modules/playwright/index.mjs';
import {decode} from '../../../frontend/node_modules/@msgpack/msgpack/dist.esm/index.mjs';
import {writeFile} from 'node:fs/promises';
const browser=await chromium.launch({headless:true});
const url=process.env.PREVIEW_URL || 'http://127.0.0.1:35383';
const reports=[];
// Keep the interactive engine alive between contexts; this socket needs no preview renderer.
const keeper=await browser.newPage();await keeper.goto(url);
try{
 for(const mode of ['volume','glyph','voxel','2D']){
  const context=await browser.newContext({viewport:{width:1640,height:1200}});
  const page=await context.newPage();const errors=[];let releaseMain=false, allowFrames=false, delivered=false, frames=0,queued=[],main;
  page.on('pageerror',e=>errors.push(e.message));
  await page.route('**/_app/immutable/chunks/*.js', async route=>{
   const response=await route.fetch(); const body=await response.text();
   if(body.includes('Field studio'))await new Promise(resolve=>setTimeout(resolve,700));
   await route.fulfill({response,body});
  });page.on('console',m=>{if(m.type()==='error')errors.push(m.text())});
  await page.routeWebSocket(u=>u.pathname==='/ws',ws=>{
   main=ws;const server=ws.connectToServer();server.onMessage(message=>{if(releaseMain)ws.send(message);else queued.push(message)});
  });
  await page.routeWebSocket(u=>u.pathname==='/ws/preview',ws=>{
   const server=ws.connectToServer();server.onMessage(message=>{
    const frame=decode(new Uint8Array(message));
    if((frame.vectorCount>0 || frame.scalarField?.length>0) && (!delivered || allowFrames)) {delivered=true;frames++;ws.send(message)}
    else if(frame.sequence) server.send(JSON.stringify({ack:frame.sequence,revision:frame.topologyRevision}));
   });
  });
  await page.addInitScript(({mode})=>localStorage.setItem('preview3d_render_mode',mode==='2D'?'volume':mode),{mode});
  const component=mode==='2D'?'x':'3D';
  for(const [endpoint,data] of [['component',{component}],['allLayers',{allLayers:true}]]){
   const r=await page.request.post(url+'/api/preview/'+endpoint,{data});if(!r.ok())throw Error(await r.text());
  }
  for(const phase of ['first-load','reload']){
   releaseMain=false;allowFrames=false;delivered=false;queued=[];main=null;
   if(phase==='first-load')await page.goto(url);else await page.reload();
   await page.locator('.preview-wrapper__stats').waitFor();await page.waitForTimeout(250);
   if(!delivered)throw Error('No first data frame');
   // Assert no renderer starts against unknown geometry.
   if(process.env.SKIP_PREMESH_GUARD!=='1' && await page.locator('#container canvas').count())throw Error(mode+' '+phase+': renderer initialized before mesh');
   releaseMain=true;for(const message of queued)main.send(message);queued=[];
   await page.locator('#container canvas').first().waitFor();await page.evaluate(()=>document.fonts.ready);await page.waitForTimeout(400);
   const before=await page.locator('#container').screenshot({path:'/tmp/3-startup-'+mode+'-'+phase+'.png'});
   allowFrames=true;
   const priorFrames=frames;
   const r=await page.request.post(url+'/api/preview/refresh',{data:{}});if(!r.ok())throw Error(await r.text());
   for(let i=0;i<60 && frames<=priorFrames;i++)await page.waitForTimeout(50);
   if(frames<=priorFrames)throw Error('Forced refresh did not deliver a new frame');
   await page.waitForTimeout(400);
   const after=await page.locator('#container').screenshot();
   if(!before.equals(after))throw Error(mode+' '+phase+': initial view differs after a forced refresh');
   reports.push({mode,phase,frames,canvas:true,stable:true});
  }
  if(errors.length)throw Error(errors.join('\n'));await context.close();
 }
 await writeFile('/tmp/3-initial-preview-result.json',JSON.stringify({reports},null,2));
 console.log('PASS initial/reload frames, delayed mesh, stable 2D/Volume/Arrows/Voxel:',JSON.stringify(reports));
}finally{await browser.close()}
