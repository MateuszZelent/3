import {chromium} from '../../../frontend/node_modules/playwright/index.mjs';
import {writeFile} from 'node:fs/promises';
const browser=await chromium.launch({headless:true});
const page=await browser.newPage({viewport:{width:1640,height:1200}});
const errors=[],reports=[];page.on('pageerror',e=>errors.push(e.message));
const url=process.env.PREVIEW_URL || 'http://127.0.0.1:35380';
async function verify(label){
 const colors=await page.evaluate(()=>{
  const s=getComputedStyle(document.documentElement);
  const token=k=>s.getPropertyValue(k).trim();
  const style=q=>{const e=document.querySelector(q);const c=e&&getComputedStyle(e);return c?{background:c.backgroundColor,image:c.backgroundImage,color:c.color}:null};
  return {surface1:token('--surface-1'),surface2:token('--surface-2'),wrapper:style('.preview-wrapper'),heading:style('.surface-heading'),window:style('.region-controls'),settings:style('.studio-settings'),panel:style('[data-panel="preview"]')};
 });
 const rgb=hex=>`rgb(${parseInt(hex.slice(1,3),16)}, ${parseInt(hex.slice(3,5),16)}, ${parseInt(hex.slice(5,7),16)})`;
 if(colors.wrapper.background!==rgb(colors.surface1) || colors.window.background!==rgb(colors.surface1) || colors.heading.background!==rgb(colors.surface2) || colors.settings.background!==rgb(colors.surface2))throw Error(label+' theme mismatch '+JSON.stringify(colors));
 reports.push({label,colors});
 await page.locator('#container').screenshot({path:'/tmp/3-ui-style-'+label+'-canvas.png'});
 await page.locator('[data-panel="preview"]').screenshot({path:'/tmp/3-ui-style-'+label+'.png'});
}
try{
 await page.goto(url);await page.getByRole('button',{name:'Volume',exact:true}).click();await page.waitForTimeout(800);
 await verify('default');
 await page.evaluate(()=>{const r=document.documentElement;r.style.setProperty('--surface-1','#e8eef6');r.style.setProperty('--surface-2','#f5f7fa');r.style.setProperty('--surface-3','#cbd5e1');r.style.setProperty('--surface-glass','#e8eef6');r.style.setProperty('--text-1','#172033');r.style.setProperty('--text-2','#334155');r.style.setProperty('--text-3','#475569');});
 await page.getByRole('button',{name:'2D section',exact:true}).click();await page.waitForTimeout(500);
 await page.getByRole('button',{name:'3D volume',exact:true}).click();await page.waitForTimeout(800);
 await verify('token-override');
 await page.setViewportSize({width:390,height:800});await page.waitForTimeout(200);
 if(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth+1))throw Error('Mobile horizontal overflow');
 if(errors.length)throw Error(errors.join('\n'));
 await writeFile('/tmp/3-ui-style-result.json',JSON.stringify({reports,errors},null,2));
 console.log('PASS computed CSS matches template tokens, canvas captures, mobile layout');
}finally{await browser.close()}
