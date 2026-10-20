import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import vm from 'node:vm';
const html=await fs.readFile(new URL('../../go-server/install.html',import.meta.url),'utf8');
const script=html.match(/<script nonce="[^]*?">([^]*?)<\/script>/)[1].replaceAll('INSTALL_ENDPOINT_PLACEHOLDER','/install/test-token').replaceAll('ADMIN_ENTRY_PLACEHOLDER','/abcdef');
async function submit(responses, action='test'){
 const status={textContent:''}, buttons=[{disabled:false},{disabled:false}], values={publicOrigin:'https://example.test',host:'127.0.0.1',port:'3306',database:'isolated',username:'owner',password:'fixture',admin:'owner',adminPassword:'1',confirm:'1'};
 const form={elements:{publicOrigin:{}},reportValidity:()=>true,querySelectorAll:()=>buttons,addEventListener(){}};
 const element={addEventListener(){}};let redirect=null;const requests=[];
 const context=vm.createContext({document:{getElementById:id=>id==='install'?form:id==='status'?status:element,querySelectorAll:()=>[]},location:{origin:'https://example.test',replace:url=>redirect=url},FormData:class{constructor(){return Object.entries(values)}},fetch:async(url,options)=>{requests.push({url,options});if(!responses.length)throw Error('unexpected request');return responses.shift()}});
 vm.runInContext(script,context);await context.send(action);assert.ok(buttons.every(b=>!b.disabled));return {status:status.textContent,redirect,requests};
}
test('installer sends the chosen short password and follows successful install',async()=>{const result=await submit([new Response(JSON.stringify({ok:true,redirect:'/abcdef#/pages/studio/index'}))],'complete');assert.equal(JSON.parse(result.requests[0].options.body).adminPassword,'1');assert.equal(result.redirect,'/abcdef#/pages/studio/index')});
test('stale installer detects completed install after plain 404 and opens login',async()=>{const result=await submit([new Response('404 page not found',{status:404}),new Response(JSON.stringify({status:'ok'}))]);assert.equal(result.redirect,'/abcdef#/pages/studio/index');assert.match(result.status,/安装已经完成/)});
test('expired install token before setup gives actionable error',async()=>{const result=await submit([new Response('404 page not found',{status:404}),new Response(JSON.stringify({status:'installation-required'}))]);assert.equal(result.redirect,null);assert.match(result.status,/安装入口已失效/)});
test('proxy HTML errors do not expose JSON parser errors or raw server bodies',async()=>{const result=await submit([new Response('<html>private upstream error</html>',{status:502})]);assert.match(result.status,/HTTP 502/);assert.doesNotMatch(result.status,/Unexpected|private upstream/)});
test('database diagnostics remain visible',async()=>{const result=await submit([new Response(JSON.stringify({error:'数据库认证失败'}),{status:400})]);assert.equal(result.status,'数据库认证失败')});
