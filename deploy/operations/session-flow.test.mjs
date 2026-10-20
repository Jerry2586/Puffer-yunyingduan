import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import vm from 'node:vm';
// Execute the actual page startup functions with controlled transport outcomes.
// CMS production connectivity is tested separately; these cases cover UI scheduling.
const source=await fs.readFile(new URL('../../src/pages/studio/index.vue',import.meta.url),'utf8');
const functions=['refreshSession','initialize'].map(name=>{
 const line=source.split('\n').find(line=>line.startsWith('async function '+name+'('));
 assert.ok(line,'missing startup function '+name);
 return line.replaceAll(' as any','').replaceAll(' as Error','');
}).join('\n');
function harness(overrides={}){
 const cell=value=>({value});
 const c={api:{setup:async()=>({installed:true}),state:async()=>({version:1}),health:async()=>({version:'test'}),...overrides},operationsCenter:true,disposed:false,serverSetup:cell(null),authenticated:cell(false),checking:cell(true),startupError:cell(''),error:cell(''),loginError:cell(''),serviceVersion:cell(''),receive:state=>{c.received=state;},refreshCmsStatus:()=>{c.cmsCalls++;return new Promise(()=>{});},cmsCalls:0};
 vm.createContext(c);vm.runInContext(functions,c);return c;
}
const bounded=p=>Promise.race([p,new Promise((_,reject)=>{const timer=setTimeout(()=>reject(Error('startup blocked')),500);timer.unref();})]);
test('session initialization completes while CMS and version requests remain pending',async()=>{
 const c=harness({health:()=>new Promise(()=>{})});await bounded(c.initialize());
 assert.equal(c.checking.value,false);assert.equal(c.authenticated.value,true);assert.equal(c.received.version,1);assert.equal(c.cmsCalls,1);
});
test('failed state request shows a retryable error and does not authenticate an empty workspace',async()=>{
 let failing=true;const c=harness({state:async()=>{if(failing)throw Error('state unavailable');return {version:2};}});
 await c.initialize();assert.equal(c.authenticated.value,false);assert.equal(c.startupError.value,'state unavailable');assert.equal(c.checking.value,false);assert.equal(c.cmsCalls,0);
 failing=false;await c.initialize();assert.equal(c.authenticated.value,true);assert.equal(c.startupError.value,'');assert.equal(c.received.version,2);
});
test('expired session opens login without reporting a service outage',async()=>{
 const c=harness({setup:async()=>{throw Object.assign(Error('login required'),{status:401});}});
 await c.initialize();assert.equal(c.authenticated.value,false);assert.equal(c.startupError.value,'');assert.equal(c.checking.value,false);assert.equal(c.cmsCalls,0);
});
