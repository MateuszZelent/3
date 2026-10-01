import { chromium } from '/home/kkingstoun/git/3/frontend/node_modules/playwright/index.mjs';
import { writeFile } from 'node:fs/promises';
const url=process.env.PREVIEW_URL || 'http://127.0.0.1:35378';
const browser=await chromium.launch({headless:true});
const page=await browser.newPage({viewport:{width:1640,height:1400}});
const errors=[];page.on('pageerror',e=>errors.push(e.message));
page.on('console',m=>{if(m.type()==='error')errors.push(m.text());});
const assert=(v,m)=>{if(!v)throw Error(m)};
const reports=[];
try {
 await page.goto(url);
 await page.getByRole('button',{name:'Volume',exact:true}).click();
 await page.waitForTimeout(1000);
 for(const mode of ['Arrows','Voxel','Volume']) {
  await page.getByRole('button',{name:mode,exact:true}).click();
  const color=page.getByLabel('Color by',{exact:true});await color.waitFor();
  assert(await color.isVisible(),mode+' color panel not visible by default');
  const panel=await color.boundingBox(),canvas=await page.locator('#container').boundingBox();
  assert(panel.y<canvas.y,mode+' color selector must be above canvas');
  if(mode==='Voxel'){
   const topo=page.getByRole('button',{name:'Voxel topography',exact:true});
   await topo.scrollIntoViewIfNeeded();assert(await topo.isVisible(),'topography hidden');
   await topo.click();
   assert(await topo.getAttribute('aria-pressed')==='true','topography failed to enable');
   await page.getByLabel('Topography amplitude',{exact:true}).waitFor();
   await page.getByRole('button',{name:'mX',exact:true}).click();
   await topo.click();
   for(const label of ['Voxel opacity','Voxel spacing','Voxel threshold']) {
    const slider=page.getByLabel(label,{exact:true});await slider.scrollIntoViewIfNeeded();assert(await slider.isVisible(),label+' hidden');
   }
   await color.scrollIntoViewIfNeeded();
  }
  const shots={};
  for(const value of ['orientation','x','y','z','magnitude']){
   await color.selectOption(value);await page.waitForTimeout(250);
   assert(await color.inputValue()===value,mode+' selector '+value);
   shots[value]=await page.locator('#container').screenshot();
  }
  assert(!shots.x.equals(shots.z),mode+' X and Z colors identical');
  assert(!shots.orientation.equals(shots.magnitude),mode+' orientation and magnitude identical');
  await page.getByRole('button',{name:'Manual',exact:true}).click();
  await page.getByLabel('3D color minimum',{exact:true}).fill('0');
  await page.getByLabel('3D color maximum',{exact:true}).fill('2');
  await page.waitForTimeout(200);
  const manual=await page.locator('#container').screenshot();
  assert(!manual.equals(shots.magnitude),mode+' manual range did not change colors');
  await page.getByRole('button',{name:'Automatic',exact:true}).click();
  reports.push({mode,vectorModes:5,manual:true});
 }
 await page.getByRole('button',{name:'Arrows',exact:true}).click(); await page.setViewportSize({width:390,height:800});
 await page.waitForTimeout(200);
 assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),'mobile overflow');
 assert(await page.getByLabel('Color by',{exact:true}).isVisible(),'mobile arrow colors collapsed');
 assert(errors.length===0,errors.join('\n'));
 await writeFile('/tmp/3-visible-panels-result.json',JSON.stringify({reports,errors},null,2));
 console.log('PASS:',JSON.stringify(reports));
}finally{await browser.close();}
