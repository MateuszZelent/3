import {chromium} from '../../../frontend/node_modules/playwright/index.mjs';
import {writeFile} from 'node:fs/promises';
const browser=await chromium.launch({headless:true});
const page=await browser.newPage({viewport:{width:1640,height:1100}});
const url='http://127.0.0.1:35379', errors=[],states=[];
page.on('pageerror',e=>errors.push(e.message));
page.on('response',async r=>{if(r.url().endsWith('/api/demag/progress') && r.ok())try{states.push(await r.json())}catch{}});
try {
 await page.goto(url); await page.getByLabel('Quantity',{exact:true}).waitFor();
 const work=page.request.post(url+'/api/console/command',{data:{command:'SaveAs(B_demag,"ui_demag_field")'},timeout:120000});
 const panel=page.getByRole('complementary',{name:'Demagnetization initialization'});
 await panel.waitFor({timeout:30000});
 await page.waitForFunction(()=>document.querySelector('.demag-progress pre')?.textContent.includes('Demag'),null,{timeout:30000});
 await panel.screenshot({path:'/tmp/3-ui-demag-progress.png'});
 const r=await work;if(!r.ok())throw Error(await r.text());
 await page.waitForTimeout(600);
 const active=states.filter(s=>s.active);
 if(active.length<2)throw Error('No live progress updates during engine work');
 if(!states.some(s=>s.id>0&&!s.active&&!s.failed))throw Error('No successful completion');
 if(errors.length)throw Error(errors.join('\n'));
 await writeFile('/tmp/3-ui-demag-result.json',JSON.stringify({activeUpdates:active.length,stages:[...new Set(active.map(s=>s.stage))],completed:states.at(-1),errors},null,2));
 console.log('PASS live demag progress',active.length,'updates');
} finally {await browser.close()}
