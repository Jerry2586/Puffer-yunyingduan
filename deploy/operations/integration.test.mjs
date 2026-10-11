import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';
import path from 'node:path';
import http from 'node:http';
import net from 'node:net';
import {spawn} from 'node:child_process';
import {generateKeyPairSync,sign,createHash} from 'node:crypto';
import {deploymentConfig} from './configure.mjs';
import {verifyBundle} from './install.mjs';
import {installAdapter} from './install-cms-adapter.mjs';
import {discoverCms,publicSiteMetadata} from '../../server/cms-discovery.mjs';
import {operationConfig,mergeOperations} from '../../server/operations.mjs';
import {defaultTheme} from '../../server/theme.mjs';
const info=JSON.parse(await fs.readFile(process.env.HETUN_OPERATIONS_BUILD_INFO,'utf8'));
const verification=path.join(path.dirname(process.env.HETUN_OPERATIONS_BUILD_INFO),'verification','server-'+Date.now());await fs.mkdir(verification,{recursive:true});
const input={domain:'ops.example.com',code:'/opt/hetun-operations/releases/0.2.0',cert:'/etc/ssl/ops/fullchain.pem',key:'/etc/ssl/ops/privkey.pem'};
test('独立部署配置及路径/域名/环境变量注入拒绝',()=>{
 const r=deploymentConfig({...input,cmsRoots:'/www/wwwroot:/var/www',cmsBase:'http://127.0.0.1:8099',allowPrivate:'1',clientOrigins:'https://client.example.com'});
 assert.match(r.env,/HETUN_ALLOW_PRIVATE_CMS=1/);assert.ok(r.env.includes("HETUN_CMS_ROOTS=/www/wwwroot:/var/www"));assert.match(r.service,/ProtectSystem=strict/);assert.match(r.nginx,/127\.0\.0\.1:8788/);
 for(const bad of [{domain:'ops.example.com; x'},{state:input.code+'/data'},{config:'/var/lib/hetun-operations/config'},{node:'/bin/node\nExecStart=/bin/sh'},{license:'http://license.example.com'},{port:80},{cmsRoots:'/www/..'},{cmsBase:'https://u:p@example.com'},{allowPrivate:'yes'},{clientOrigins:'https://client.example.com/path'}])assert.throws(()=>deploymentConfig({...input,...bad}));
});
test('发布包校验且不携带依赖、凭据及运行数据',async()=>{const manifest=await verifyBundle(info.bundle);assert.equal(manifest.version,'0.2.0');for(const f of Object.keys(manifest.files))assert.doesNotMatch(f,/node_modules|admin-credentials|theme-state|cloud-license/);assert.ok(manifest.files['install.mjs']);assert.ok(manifest.files['server/cms-discovery.mjs']);});
test('运营配置保留打包品牌和视觉，轮播/模块配置真实合并',()=>{const packaged=structuredClone(defaultTheme);packaged.brand.name='已打包品牌';packaged.colors.accent='#112233';const theme=structuredClone(defaultTheme);theme.carousel.style='peek';theme.carousel.height=320;const config=operationConfig(theme);assert.equal(config.brand,undefined);assert.equal(config.colors,undefined);const merged=mergeOperations(config,packaged);assert.equal(merged.brand.name,'已打包品牌');assert.equal(merged.colors.accent,'#112233');assert.equal(merged.carousel.style,'peek');assert.equal(merged.carousel.height,320);});
const listen=s=>new Promise((resolve,reject)=>{s.once('error',reject);s.listen(0,'127.0.0.1',()=>resolve(s.address().port));});
const close=s=>new Promise(resolve=>{s.closeAllConnections?.();s.close(resolve);});
async function freePort(){const s=net.createServer(),p=await listen(s);await close(s);return p;}
async function stop(child){if(child.exitCode!==null)return;await new Promise(resolve=>{const timeout=setTimeout(()=>{child.kill('SIGKILL');resolve();},3000);child.once('exit',()=>{clearTimeout(timeout);resolve();});child.kill();});}
async function fixture(root,name,url){const dir=path.join(root,name);await fs.mkdir(path.join(dir,'application/extra'),{recursive:true});await fs.mkdir(path.join(dir,'application/index/controller'),{recursive:true});await fs.writeFile(path.join(dir,'api.php'),'<?php');await fs.writeFile(path.join(dir,'application/extra/maccms.php'),"<?php return ['site_name'=>'隔离测试站点','site_url'=>'"+url+"','db_password'=>'secret-not-exported'];");return dir;}
test('站点识别只读取公开字面配置、多个站点不误绑、适配器不覆盖已有文件',async()=>{const dir=await fs.mkdtemp(path.join(verification,'discovery-'));const a=await fixture(dir,'a','https://a.example.com'),b=await fixture(dir,'b','https://b.example.com');const result=await discoverCms({roots:[dir],check:async()=>({ok:true})});assert.equal(result.candidates.length,2);assert.equal(result.recommendedId,'');assert.equal(result.multiple,true);assert.doesNotMatch(JSON.stringify(result),/secret-not-exported/);assert.deepEqual(publicSiteMetadata("'site_name'=>'x','db_pass'=>'y'"),{name:'x',url:'',installDir:''});const source=path.join(info.bundle,'server/maccms-extension/Hetun.php');assert.equal((await installAdapter(a,source)).installed,true);assert.equal((await installAdapter(a,source)).unchanged,true);await fs.writeFile(path.join(b,'application/index/controller/Hetun.php'),'existing user adapter');await assert.rejects(()=>installAdapter(b,source),/未覆盖/);assert.equal(await fs.readFile(path.join(b,'application/index/controller/Hetun.php'),'utf8'),'existing user adapter');});
test('打包后的真实HTTP服务：初始化、签名授权、四端独立发布、权限与重启持久化',async t=>{
 const state=await fs.mkdtemp(path.join(verification,'state-'));
 const {privateKey,publicKey}=generateKeyPairSync('ed25519');const trusted=path.join(state,'trusted-public.pem');await fs.writeFile(trusted,publicKey.export({type:'spki',format:'pem'}));
 let cloudOrigin;const cloud=http.createServer(async(req,res)=>{let raw='';for await(const b of req)raw+=b;const q=JSON.parse(raw);if(q.scope!=='film-center'){res.writeHead(403,{'Content-Type':'application/json'});res.end(JSON.stringify({error:'隔离授权仅提供film-center'}));return;}const now=Math.floor(Date.now()/1000),payload=Buffer.from(JSON.stringify({issuer:cloudOrigin,scope:q.scope,domain:q.domain,nonce:q.nonce,iat:now,exp:now+120,licenseId:'isolated-test-only',licenseExpiresAt:new Date(Date.now()+86400000).toISOString(),themes:['*']}));res.setHeader('Content-Type','application/json');res.end(JSON.stringify({payload:payload.toString('base64url'),signature:sign(null,payload,privateKey).toString('base64url')}));});cloudOrigin='http://127.0.0.1:'+await listen(cloud);t.after(()=>close(cloud));
 const cms=http.createServer((_req,res)=>{res.setHeader('Content-Type','application/json');res.end(JSON.stringify({code:1,page:1,pagecount:1,limit:20,total:1,class:[{type_id:1,type_name:'隔离测试分类'}],list:[{vod_id:1,vod_name:'隔离接口验收影片',type_id:1,type_name:'隔离测试分类',vod_play_url:'不可泄漏的播放地址'}]}));});const cmsOrigin='http://127.0.0.1:'+await listen(cms);t.after(()=>close(cms));
 const roots=path.join(state,'sites');await fixture(roots,'cms',cmsOrigin);
 const port=await freePort(),api='http://127.0.0.1:'+port,origin='https://ops.example.com';let child,logs='';
 const env={...process.env,HETUN_STATE_DIR:state,HETUN_HOST:'127.0.0.1',HETUN_PORT:String(port),HETUN_PUBLIC_ORIGIN:origin,HETUN_PRODUCT_MODE:'operations-center',HETUN_LOCAL_OPERATIONS:'0',HETUN_LICENSE_DOMAIN:'ops.example.com',HETUN_LICENSE_URL:cloudOrigin,HETUN_LICENSE_PUBLIC_KEY_FILE:trusted,HETUN_ALLOW_PRIVATE_CMS:'1',HETUN_CMS_ROOTS:roots,HETUN_FRONTEND_DIR:path.join(info.bundle,'frontend')};
 async function boot(){child=spawn(process.execPath,[path.join(info.bundle,'server/index.mjs')],{env,windowsHide:true,stdio:['ignore','pipe','pipe']});child.stdout.on('data',v=>logs+=v);child.stderr.on('data',v=>logs+=v);for(let i=0;i<100;i++){if(child.exitCode!==null)throw Error('服务退出：'+logs);try{if((await fetch(api+'/api/health')).ok)return;}catch{}await new Promise(r=>setTimeout(r,50));}throw Error('服务启动超时：'+logs);}
 t.after(async()=>{if(child)await stop(child);});await boot();
 const credentials=JSON.parse(await fs.readFile(path.join(state,'admin-credentials.json'),'utf8'));let headers={Origin:origin,'X-Hetun-Client':'studio','Content-Type':'application/json'};
 const call=(endpoint,method='GET',data)=>fetch(api+endpoint,{method,headers,body:data===undefined?undefined:JSON.stringify(data)});
 assert.equal((await call('/api/admin/setup')).status,401);assert.equal((await fetch(api+'/api/runtime')).status,503);assert.equal((await fetch(api+'/')).status,200);
 let r=await fetch(api+'/api/admin/login',{method:'POST',headers:{...headers,Origin:'https://foreign.example.com'},body:JSON.stringify(credentials)});assert.equal(r.status,403);
 r=await call('/api/admin/login','POST',{username:credentials.username,password:credentials.password});assert.equal(r.status,200);assert.match(r.headers.get('set-cookie'),/HttpOnly/);assert.match(r.headers.get('set-cookie'),/Secure/);headers.Cookie=r.headers.get('set-cookie').split(';')[0];
 assert.equal((await (await call('/api/admin/setup')).json()).installed,false);
 r=await call('/api/admin/setup/discover','POST',{});assert.equal(r.status,200);const discovery=await r.json();assert.equal(discovery.candidates.length,1);assert.ok(discovery.recommendedId);assert.equal(discovery.candidates[0].report.ok,true);assert.doesNotMatch(JSON.stringify(discovery),/secret-not-exported/);
 r=await call('/api/admin/setup/complete','POST',{username:'owner',password:'weak',base:cmsOrigin});assert.equal(r.status,400);
 r=await call('/api/admin/setup/complete','POST',{username:'owner',password:'ValidPass12345',base:'http://127.0.0.1:1'});assert.equal(r.status,400);
 r=await call('/api/admin/setup/complete','POST',{username:'owner',password:'ValidPass12345',base:cmsOrigin});assert.equal(r.status,200);assert.equal((await r.json()).installed,true);const after=JSON.parse(await fs.readFile(path.join(state,'admin-credentials.json'),'utf8'));assert.equal(after.password,undefined);assert.equal(after.username,'owner');
 assert.equal((await call('/api/admin/setup/complete','POST',{})).status,409);assert.equal((await call('/api/admin/state')).status,403);assert.equal((await fetch(api+'/api/runtime')).status,403);
 r=await call('/api/admin/license','PUT',{key:'HTN_'+ 'isolated_test_'.repeat(4)});assert.equal(r.status,200);
 r=await call('/api/admin/state');assert.equal(r.status,200);const initial=await r.json();assert.equal(initial.templates.length,4);assert.equal(initial.selectedTemplate,'runtime-mobile');
 assert.equal((await call('/api/admin/install','POST',{})).status,404);
 const expected={};for(const p of ['mobile','tablet','tv','web']){
  const current=await (await call('/api/admin/state?template=runtime-'+p)).json();const theme=structuredClone(current.draft);theme.sections[0].title=p+'独立模块';theme.carousel.style='peek';theme.carousel.height=310;theme.navigation[0].label=p+'首页';if(p==='tv')theme.features.member=false;
  r=await call('/api/admin/draft','PUT',{template:'runtime-'+p,version:current.version,theme});assert.equal(r.status,200);const saved=await r.json();assert.equal(saved.version,current.version+1);assert.equal(saved.draft.carousel.style,'peek');
  assert.equal((await call('/api/admin/draft','PUT',{template:'runtime-'+p,version:current.version,theme})).status,409);
  const before=await (await fetch(api+'/api/runtime?platform='+p)).json();assert.notEqual(before.config.sections[0].title,theme.sections[0].title);
  assert.equal((await call('/api/admin/publish','POST',{template:'runtime-'+p,version:current.version})).status,409);
  r=await call('/api/admin/publish','POST',{template:'runtime-'+p,version:saved.version});assert.equal(r.status,200);
  const runtime=await (await fetch(api+'/api/runtime?platform='+p)).json();assert.equal(runtime.config.sections[0].title,p+'独立模块');assert.equal(runtime.config.navigation[0].label,p+'首页');assert.equal(runtime.config.brand,undefined);assert.equal(runtime.config.colors,undefined);assert.equal(runtime.config.carousel.style,'peek');expected[p]=runtime.config;
 }
 assert.equal((await fetch(api+'/api/runtime?platform=invalid')).status,400);r=await fetch(api+'/api/catalog/categories');assert.equal(r.status,200);assert.equal((await r.json()).items[0].id,'1');
 assert.equal((await fetch(api+'/api/member/config',{headers:{'X-Hetun-Platform':'tv'}})).status,403);
 assert.equal((await fetch(api+'/api/admin/draft',{method:'PUT',headers:{...headers,Origin:'https://foreign.example.com'},body:'{}'})).status,403);
 assert.equal((await fetch(api+'/api/admin/account',{method:'PUT',headers,body:JSON.stringify({currentPassword:'wrong',username:'owner',password:'OtherPass12345'})})).status,403);
 r=await call('/api/admin/account','PUT',{currentPassword:'ValidPass12345',username:'owner2',password:'OtherPass12345'});assert.equal(r.status,200);
 await stop(child);await boot();for(const p of ['mobile','tablet','tv','web'])assert.deepEqual((await (await fetch(api+'/api/runtime?platform='+p)).json()).config,expected[p]);
 r=await call('/api/admin/login','POST',{username:'owner2',password:'OtherPass12345'});assert.equal(r.status,200);
 const events=(await fs.readFile(path.join(state,'audit.jsonl'),'utf8')).trim().split('\n').map(JSON.parse);assert.ok(events.some(e=>e.event==='setup.complete'));assert.ok(events.some(e=>e.event==='account.changed'));assert.doesNotMatch(JSON.stringify(events),/ValidPass|OtherPass|secret-not-exported/);
 await fs.writeFile(path.join(verification,'service-acceptance.json'),JSON.stringify({bundle:info.bundle,isolatedCmsFixture:true,realHttpChecks:true,filmCenterOnlyGrant:true,productionLicenseEnforced:true,fourPlatformPersistence:true,carouselChangesPersist:true,noTemplateImport:true,plaintextCredentialRemoved:true,productionDeployment:false},null,2));
});
