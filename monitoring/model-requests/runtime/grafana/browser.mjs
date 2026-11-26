// One native remote Chrome instance. No local browser, npm tree or mocked page.
import fs from 'node:fs/promises';
import {spawn} from 'node:child_process';
import assert from 'node:assert/strict';
import path from 'node:path';

const root = path.dirname(new URL(import.meta.url).pathname);
process.chdir(root);
const config = JSON.parse(await fs.readFile('browser-config.json', 'utf8'));
const fixture = JSON.parse(await fs.readFile('fixture.json', 'utf8'));
const profile = await fs.mkdtemp(path.join(root, 'chrome-profile-'));
const chromeLog = await fs.open('chrome.log', 'w');
const chrome = spawn('/Applications/Google Chrome.app/Contents/MacOS/Google Chrome', [
  '--headless=new', '--window-size=1600,1800', '--force-device-scale-factor=1', '--remote-debugging-port=0', '--remote-debugging-address=127.0.0.1',
  `--user-data-dir=${profile}`, '--no-first-run', '--no-default-browser-check',
  '--disable-background-networking', '--disable-extensions', '--disable-background-timer-throttling', '--disable-renderer-backgrounding',
  '--renderer-process-limit=2', '--js-flags=--max-old-space-size=256', 'about:blank',
], {stdio: ['ignore', chromeLog.fd, chromeLog.fd]});
await fs.writeFile('native-pids.json', JSON.stringify({node:process.pid, chrome:chrome.pid}));
const delay = ms => new Promise(resolve => setTimeout(resolve, ms));
async function until(fn, label, timeout = 60000) {
  const start = Date.now();
  while (Date.now() - start < timeout) { const value = await fn(); if (value) return value; await delay(250); }
  throw new Error(`Timed out: ${label}`);
}
let socket;
const pending = new Map();
let serial = 0;
const requests = new Map();
const captured = [];
const errors = [];
const bodyPromises = [];
const checks = [];
function check(name, value, detail) { checks.push({name, passed: !!value, detail}); assert(value, name); }
try {
  const endpoint = await until(async () => {
    try { const text = await fs.readFile(path.join(profile, 'DevToolsActivePort'), 'utf8'); return `http://127.0.0.1:${text.split('\n')[0]}`; } catch { return null; }
  }, 'Chrome debugging endpoint');
  const tabs = await (await fetch(endpoint + '/json/list')).json();
  socket = new WebSocket(tabs.find(tab => tab.type === 'page').webSocketDebuggerUrl);
  await new Promise((resolve, reject) => { socket.addEventListener('open', resolve, {once: true}); socket.addEventListener('error', reject, {once: true}); });
  function cdp(method, params = {}) {
    return new Promise((resolve, reject) => { const id = ++serial; pending.set(id, {resolve, reject}); socket.send(JSON.stringify({id, method, params})); });
  }
  socket.addEventListener('message', event => {
    const item = JSON.parse(event.data);
    if (item.id) { const waiter = pending.get(item.id); pending.delete(item.id); if (item.error) waiter?.reject(new Error(JSON.stringify(item.error))); else waiter?.resolve(item.result); return; }
    const p = item.params;
    if (item.method === 'Network.requestWillBeSent' && p.request.url.includes('/api/ds/query')) requests.set(p.requestId, {url: p.request.url, post: p.request.postData ? JSON.parse(p.request.postData) : null});
    if (item.method === 'Network.loadingFinished' && requests.has(p.requestId)) {
      const record = requests.get(p.requestId); requests.delete(p.requestId);
      bodyPromises.push(cdp('Network.getResponseBody', {requestId: p.requestId}).then(result => { captured.push({...record, body: JSON.parse(result.body)}); }).catch(error => errors.push({type: 'queryCapture', message: String(error)})));
    }
    if (item.method === 'Runtime.exceptionThrown') errors.push({type: 'browserException', message: p.exceptionDetails.text});
  });
  const evaluate = async expression => {
    const result = await cdp('Runtime.evaluate', {expression, returnByValue: true, awaitPromise: true});
    if (result.exceptionDetails) throw new Error(JSON.stringify(result.exceptionDetails));
    return result.result.value;
  };
  await cdp('Page.enable'); await cdp('Network.enable'); await cdp('Runtime.enable');
  const windowInfo=await cdp('Browser.getWindowForTarget');
  await cdp('Browser.setWindowBounds',{windowId:windowInfo.windowId,bounds:{width:1600,height:1800,windowState:'normal'}});
  await cdp('Page.bringToFront');
  await cdp('Emulation.setFocusEmulationEnabled', {enabled:true});
  await cdp('Emulation.setDeviceMetricsOverride', {width: 1600, height: 1800, screenWidth:1600, screenHeight:1800, deviceScaleFactor: 1, mobile: false});
  await cdp('Emulation.setTimezoneOverride', {timezoneId: 'Asia/Shanghai'});
  async function shot(name) {
    await cdp('Page.bringToFront');
    const result = await cdp('Page.captureScreenshot', {format:'png', fromSurface:true, captureBeyondViewport:false});
    await fs.writeFile(name+'.png',Buffer.from(result.data,'base64'));
    const native=await cdp('Page.captureScreenshot',{format:'png',fromSurface:false,captureBeyondViewport:false});
    await fs.writeFile(name+'-native.png',Buffer.from(native.data,'base64'));
  }
  await cdp('Page.navigate', {url: config.base + '/login'});
  await until(() => evaluate('!!document.querySelector("input[type=password]")'), 'real Grafana login form');
  await shot('01-login');
  const fields = await evaluate('[...document.querySelectorAll("input")].map(x=>({name:x.name,type:x.type,placeholder:x.placeholder}))');
  await fs.writeFile('login-fields.json', JSON.stringify(fields, null, 2));
  const password = (await fs.readFile('password', 'utf8')).trim();
  await evaluate(`(() => {
    const inputs=[...document.querySelectorAll('input')];
    const user=inputs.find(x=>x.type==='text'||x.type==='email');
    const password=inputs.find(x=>x.type==='password');
    const set=Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value').set;
    for(const [input,value] of [[user,${JSON.stringify(config.username)}],[password,${JSON.stringify(password)}]]) {set.call(input,value);input.dispatchEvent(new Event('input',{bubbles:true}));input.dispatchEvent(new Event('change',{bubbles:true}));}
    document.querySelector('button[type=submit]').click();
  })()`);
  await until(() => evaluate('!location.pathname.startsWith("/login")'), 'login redirect');
  check('real browser authenticated login', true);

  async function dashboard(suffix, name, expectData = true) {
    const before = captured.length;
    await cdp('Page.navigate', {url: config.base + config.dashboard + suffix});
    await until(() => evaluate('document.body.innerText.includes("整体模型请求成功率")'), 'dashboard content');
    await until(() => captured.length >= before + 7, 'seven browser-originated panel query responses', 90000);
    await Promise.all(bodyPromises);
    await delay(2000);
    await evaluate('new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)))');
    const dom = await evaluate('({text:document.body.innerText,canvases:[...document.querySelectorAll("canvas")].map(x=>({width:x.width,height:x.height,rect:x.getBoundingClientRect().toJSON(),visibility:getComputedStyle(x).visibility})),sections:[...document.querySelectorAll("[aria-label]")].map(x=>({tag:x.tagName,label:x.getAttribute("aria-label")})).filter(x=>x.label.includes("成功率")||x.label.includes("调用"))})');
    await fs.writeFile(name + '-dom.json', JSON.stringify(dom, null, 2));
    await shot(name);
    const current = captured.slice(before);
    if (expectData) check(name + ' real panels rendered', dom.canvases.filter(x=>x.width>0&&x.height>0).length >= 6, dom.canvases);
    check(name + ' no panel query errors', current.every(x => Object.values(x.body.results ?? {}).every(r => !r.error)), current.map(x=>Object.keys(x.body.results ?? {})));
    return {dom, current};
  }
  const finalDashboard = config.dashboard;
  config.dashboard = '/d/wujie-model-requests-baseline';
  const baseline = await dashboard('', '02-baseline-hours');
  config.dashboard = finalDashboard;
  const defaults = await dashboard('', '03-default-hours');
  const imported = JSON.parse(await fs.readFile('import-result.json','utf8')).stored.dashboard;
  check('percent panels keep percentunit', imported.panels.filter(x=>[2,3,7].includes(x.id)).every(x=>x.fieldConfig.defaults.unit==='percentunit'));
  check('count panels keep short unit', imported.panels.filter(x=>[4,5,6,8].includes(x.id)).every(x=>x.fieldConfig.defaults.unit==='short'));
  check('source default rendered wujieai', defaults.dom.text.includes('wujieai'));
  check('model default rendered All', defaults.dom.text.includes('All'));
  for (const response of defaults.current) {
    check('query bounds select exactly the 24 bucket endpoints', Number(response.post.from)===(fixture.hour-23*3600)*1000 && Number(response.post.to)===fixture.hour*1000+999, {from:response.post.from,to:response.post.to});
    const times = Object.values(response.body.results ?? {}).flatMap(r=>(r.frames ?? []).flatMap(f=>f.schema.fields.flatMap((field,i)=>field.type==='time'?f.data.values[i]:[])));
    check('plotted buckets on complete-hour boundaries', times.every(t=>t % 3600000===0 && t<=fixture.hour*1000), times);
    check('browser fixed one-hour query step', response.post.queries.every(x=>x.intervalMs === 3600000), response.post.queries.map(x=>x.intervalMs));
  }
  // Keep observed browser responses and draw evidence; assertions below read those
  // responses rather than querying Grafana/Prometheus separately.
  function samples(responses, fragment, label) {
    const frames = responses.filter(x=>x.post.queries.some(q=>q.expr.includes(fragment))).flatMap(x=>Object.values(x.body.results).flatMap(r=>r.frames ?? []));
    return frames.filter(f=>!label || f.schema.fields.some(field=>field.labels?.model===label)).flatMap(frame=>{
      const ti=frame.schema.fields.findIndex(x=>x.type==='time');
      return frame.schema.fields.flatMap((field,index)=>field.type==='number'?frame.data.values[index].map((value,i)=>({time:frame.data.values[ti][i],value,labels:field.labels ?? {}})):[]);
    });
  }
  function valueAt(responses, fragment, hour, label) {return samples(responses, fragment, label).filter(x=>x.time===hour*1000&&x.value!==null).map(x=>x.value);}
  const countFragment='sum(increase(wujie_model_requests_total';
  const countTimes=responses=>[...new Set(samples(responses,countFragment).filter(x=>x.value!==null).map(x=>x.time))].sort((a,b)=>a-b);
  const baselineTimes=countTimes(baseline.current);
  const finalTimes=countTimes(defaults.current);
  check('baseline reproduces 25 completed-hour buckets', baselineTimes.length===25 && baselineTimes[0]===(fixture.hour-24*3600)*1000, baselineTimes);
  check('corrected default returns exactly 24 complete buckets', finalTimes.length===24 && finalTimes.every((t,i)=>t===(fixture.hour-(23-i)*3600)*1000), finalTimes);
  check('latest completed hour retained and incomplete hour excluded', finalTimes.at(-1)===fixture.hour*1000 && finalTimes.every(t=>t<=fixture.hour*1000));
  const overallFragment = '((sum(increase';
  check('mixed-hour overall 30 percent', valueAt(defaults.current, overallFragment, fixture.mixedEnd).some(x=>Math.abs(x-.3)<.001));
  check('all-failed hour is zero percent', valueAt(defaults.current, overallFragment, fixture.failedEnd).includes(0));
  check('zero-traffic hour has no success-rate value', valueAt(defaults.current, overallFragment, fixture.emptyEnd).length===0);
  check('per-model mixed hour gpt-4 60 percent', valueAt(defaults.current, 'sum by (model)', fixture.mixedEnd, 'gpt-4').some(x=>Math.abs(x-.6)<.001));
  check('failed-only model mixed hour zero percent', valueAt(defaults.current, 'sum by (model)', fixture.mixedEnd, 'model-failed').includes(0));
  check('unknown mixed-hour share 10 percent', defaults.current.filter(x=>x.post.queries.some(q=>q.expr.includes('outcome="unknown"'))).some(r=>valueAt([r], 'outcome="unknown"', fixture.mixedEnd).some(x=>Math.abs(x-.1)<.001)));
  const linked = await dashboard('?var-source=wujieai&from=now-23h%2Fh&to=now%2Fh-1h%2B1s', '04-management-url');
  check('encoded management URL preserves24 complete buckets', JSON.stringify(countTimes(linked.current))===JSON.stringify(finalTimes));
  const filtered = await dashboard('?var-source=wujieai&var-model=model-failed', '05-failed-model');
  check('model variable selection appears', filtered.dom.text.includes('model-failed'));
  check('model selection filters real panel queries', filtered.current.every(x=>x.post.queries.every(q=>q.expr.includes('model=~"model-failed"'))));
  check('selected failed model zero percent', valueAt(filtered.current, overallFragment, fixture.mixedEnd).includes(0));
  const absent = await dashboard(`?from=${(fixture.absentEnd-3600)*1000}&to=${fixture.absentEnd*1000}&var-source=wujieai`, '06-absent-hour', false);
  check('absent-hour panels show no data', /No data|无数据/.test(absent.dom.text));
  check('absent-hour actual query frames contain no numbers', absent.current.every(x=>Object.values(x.body.results ?? {}).every(r=>(r.frames ?? []).every(f=>f.schema.fields.every((field,i)=>field.type!=='number'||f.data.values[i].every(v=>v===null))))));
  await fs.writeFile('browser-queries.json', JSON.stringify(captured, null, 2));
  await fs.writeFile('browser-result.json', JSON.stringify({browser:'native Google Chrome headless on remote Mac',version:await cdp('Browser.getVersion'),checks,errors,fixtureBoundary:'Historical values are synthetic; current-hour live gateway data is validated separately.'},null,2));
  console.log(JSON.stringify({checks:checks.length,failures:checks.filter(x=>!x.passed),browserErrors:errors},null,2));
} catch (error) {
  await fs.writeFile('browser-failure.json', JSON.stringify({error:String(error),checks,errors},null,2));
  await fs.writeFile('browser-queries.json', JSON.stringify(captured,null,2));
  throw error;
} finally {
  socket?.close(); chrome.kill('SIGTERM');
  await new Promise(resolve=>{if(chrome.exitCode!==null)resolve();else {chrome.once('exit',resolve);setTimeout(()=>{chrome.kill('SIGKILL');resolve();},5000).unref();}});
  await chromeLog.close();
}
