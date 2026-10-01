import { chromium } from '/home/kkingstoun/git/3/frontend/node_modules/playwright/index.mjs';
import { writeFile } from 'node:fs/promises';
const url=process.env.PREVIEW_URL || 'http://127.0.0.1:35377';
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
 await page.setViewportSize({width:390,height:800});
 await page.waitForTimeout(200);
 assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth+1),'mobile overflow');
 assert(errors.length===0,errors.join('\n'));
 await writeFile('/tmp/3-color-browser-result.json',JSON.stringify({reports,errors},null,2));
 console.log('PASS:',JSON.stringify(reports));
}finally{await browser.close();}
