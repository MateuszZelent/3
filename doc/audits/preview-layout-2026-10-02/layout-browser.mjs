import {chromium} from '../../../frontend/node_modules/playwright/index.mjs';
import {writeFile} from 'node:fs/promises';
const browser=await chromium.launch({headless:true});
const url=process.env.PREVIEW_URL||'http://127.0.0.1:35387';
const reports=[];
const keeper=await browser.newPage();await keeper.goto(url);
try {
 for(const viewport of [{width:1640,height:1200},{width:390,height:844}]) {
  const context=await browser.newContext({viewport});const page=await context.newPage();const errors=[];
  page.on('pageerror',e=>errors.push(e.message));
  await page.addInitScript(()=>localStorage.setItem('preview3d_render_mode','volume'));
  await page.request.post(url+'/api/preview/component',{data:{component:'3D'}});
  await page.request.post(url+'/api/preview/quantity',{data:{quantity:'m'}});
  await page.request.post(url+'/api/preview/allLayers',{data:{allLayers:true}});
  await page.goto(url);await page.locator('.preview-viewport canvas').first().waitFor();
  await page.locator('.vc').waitFor();
  if(await page.getByLabel('Color by',{exact:true}).inputValue()!=='orientation')throw Error('Wrong default color mode');
  for(const mode of ['Volume','Arrows','Voxel']) {
   await page.locator('.volume-strip').getByRole('button',{name:mode,exact:true}).click();
   for(const windowMode of ['inline','popout','fullscreen']) {
    if(windowMode==='popout')await page.locator('.preview-mode-switcher').getByRole('button',{name:'Popout',exact:true}).click();
    if(windowMode==='fullscreen')await page.locator('.preview-wrapper__title-actions').getByRole('button',{name:'Fullscreen',exact:true}).click();
    await page.waitForTimeout(250);
    if(await page.locator('.render-controls').count()!==1 || await page.locator('.region-controls').count()!==1 || await page.locator('.advanced-appearance,.voxel-controls').count())throw Error('Duplicate display controls');
    const result=await page.evaluate(()=>{
     const box=e=>{const r=e.getBoundingClientRect();return {x:r.x,y:r.y,width:r.width,height:r.height,right:r.right,bottom:r.bottom}};
     const canvas=box(document.querySelector('#container'));
     const controls=box(document.querySelector('.preview-field-controls'));
     const stats=box(document.querySelector('.preview-wrapper__stats'));
     const elements=[...document.querySelectorAll('.vc,.ag,.vc-face,.ag-lbl,.ag-shaft,.ag-tip,.vc-home')].map(e=>({class:e.className,...box(e)}));
     const inside=r=>r.x>=canvas.x-1&&r.y>=canvas.y-1&&r.right<=canvas.right+1&&r.bottom<=canvas.bottom+1;
     return {canvas,controls,stats,contained:elements.every(inside),outside:elements.filter(e=>!inside(e)),controlsClear:controls.bottom<=canvas.y+1,statsClear:stats.y>=canvas.bottom-1};
    });
    if(!result.contained||!result.controlsClear||!result.statsClear||result.canvas.height<128)throw Error(JSON.stringify({mode,windowMode,viewport,...result}));
    reports.push({mode,windowMode,viewport,...result});
    await page.locator('.preview-viewport').screenshot({path:`/tmp/3-layout-${viewport.width}-${mode}-${windowMode}.png`});
    if(windowMode==='fullscreen')await page.locator('.surface-heading').getByRole('button',{name:'Exit fullscreen',exact:true}).click();
   }
  }
  await page.locator('.volume-strip').getByRole('button',{name:'Volume',exact:true}).click();
  await page.locator('.render-controls > summary').click();
  if(await page.getByLabel('Surface lighting',{exact:true}).count()!==1)throw Error('Duplicate lighting');
  await page.locator('label.ui-toggle').filter({hasText:'Surface lighting'}).click();
  if(await page.getByLabel('Lighting intensity',{exact:true}).isDisabled())throw Error('Lighting intensity did not enable');
  if(!await page.locator('.volume-controls').evaluate(e=>e.open))await page.locator('.volume-controls > summary').click();
  await page.getByLabel('Color by',{exact:true}).selectOption('x');
  await page.waitForTimeout(150);
  await page.locator('.region-controls > summary').click();
  const updated=page.waitForResponse(r=>r.url().endsWith('/api/preview/region')&&r.request().method()==='POST');
  const firstCell=page.getByRole('spinbutton',{name:'X first cell',exact:true});
  await firstCell.fill(Number(await firstCell.inputValue())===0?'1':'0');
  await page.getByRole('spinbutton',{name:'X first cell',exact:true}).press('Tab');
  const response=await updated;if(!response.ok())throw Error(await response.text());
  await page.waitForTimeout(250);
  const restore=page.waitForResponse(r=>r.url().endsWith('/api/preview/region')&&r.request().method()==='POST');
  await page.locator('.region-controls').getByRole('button',{name:'Full domain',exact:true}).click();await restore;
  await page.locator('.volume-strip').getByRole('button',{name:'Voxel',exact:true}).click();
  await page.locator('label.ui-toggle').filter({hasText:'Voxel topography'}).click();
  await page.getByLabel('Topography component',{exact:true}).selectOption('y');
  if(await page.getByLabel('Topography amplitude',{exact:true}).count()!==1)throw Error('Missing or duplicate topography');
  await page.locator('.volume-strip').getByRole('button',{name:'Volume',exact:true}).click();
  const column=await page.evaluate(()=>{
   document.querySelector('.zone-table').style.minHeight='4000px';
   const viz=document.querySelector('.zone-viz').getBoundingClientRect(), solver=document.querySelector('.zone-solver').getBoundingClientRect();
   const expected=parseFloat(getComputedStyle(document.querySelector('.workspace-column')).rowGap);
   return {gap:solver.top-viz.bottom,expected,sameColumn:Math.abs(viz.left-solver.left)<1};
  });
  if(!column.sameColumn||Math.abs(column.gap-column.expected)>1)throw Error(JSON.stringify(column));
  reports.push({uniqueControls:true,lightingWorks:true,localWindowRequest:true,topographyWorks:true,column,viewport});
  await page.getByLabel('Quantity',{exact:true}).selectOption('alpha');
  await page.waitForTimeout(400);await page.locator('.vc').waitFor();
  reports.push({scalarCameraVisible:true,viewport});
  if(errors.length)throw Error(errors.join('\n'));
  await context.close();
 }
 await writeFile('/tmp/3-preview-layout-result.json',JSON.stringify({reports},null,2));
 console.log('PASS',reports.length,'layout/scalar-camera checks');
} finally {await browser.close()}
