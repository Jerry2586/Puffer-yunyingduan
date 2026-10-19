import test from 'node:test';
import assert from 'node:assert/strict';
import http from 'node:http';
import {initializeOperations,saveOperations,publishOperations,rollbackOperations,clientBootstrap,operationConfig,requestPlatform} from '../../server/operations.mjs';
import {defaultTheme} from '../../server/theme.mjs';
import {listCms} from '../../server/cms.mjs';
const fresh=()=>{const s={cmsBase:'https://cms.example.com'};initializeOperations(s);return s;};
test('发布和回退均生成新版本，旧窗口不能重放且不会污染状态',()=>{
 const s=fresh(),r=s.operations.mobile,theme=structuredClone(defaultTheme);theme.sections[0].title='第二版栏目';
 saveOperations(s,{version:1,theme});publishOperations(s,{version:2});assert.equal(r.version,3);assert.equal(r.publishedVersion,2);assert.equal(r.history[0].version,1);
 const before=JSON.stringify(s);assert.throws(()=>publishOperations(s,{version:2}),{status:409});assert.equal(JSON.stringify(s),before);
 rollbackOperations(s,{version:3,targetVersion:1});assert.equal(r.version,4);assert.equal(r.publishedVersion,3);assert.equal(r.published.sections[0].title,defaultTheme.sections[0].title);assert.equal(r.history[0].version,2);
 const restored=JSON.stringify(s);assert.throws(()=>rollbackOperations(s,{version:3,targetVersion:2}),{status:409});assert.equal(JSON.stringify(s),restored);
 for(let i=0;i<25;i++)publishOperations(s,{version:r.version});assert.equal(r.history.length,20);
});
test('更新按数字版本及系统分发，拒绝新HTTP更新链接，兼容读取旧数据',()=>{
 const s=fresh(),theme=structuredClone(defaultTheme);theme.clientSettings={...theme.clientSettings,latestVersion:'1.10.0',androidUrl:'https://downloads.example.com/app.apk',iosUrl:'https://apps.apple.com/app/id123',forceUpdate:true};
 saveOperations(s,{version:1,theme});publishOperations(s,{version:2});
 const android=clientBootstrap(s,'mobile',{version:'1.9.9'});assert.equal(android.update.available,true);assert.equal(android.update.required,true);assert.match(android.update.url,/app.apk$/);
 assert.equal(clientBootstrap(s,'mobile',{os:'ios',version:'1.0.0'}).update.url,theme.clientSettings.iosUrl);
 assert.equal(clientBootstrap(s,'mobile',{version:'1.10.0'}).update.available,false);assert.equal(clientBootstrap(s,'mobile',{os:'windows',version:'1.0.0'}).update.available,false);
 assert.throws(()=>clientBootstrap(s,'mobile',{version:'1.0-beta'}),{status:400});
 theme.clientSettings.androidUrl='http://downloads.example.com/app.apk';assert.throws(()=>saveOperations(s,{version:3,theme}),{status:400});assert.equal(s.operations.mobile.version,3);
 s.operations.mobile.published.clientSettings.androidUrl=theme.clientSettings.androidUrl;initializeOperations(s);assert.equal(clientBootstrap(s,'mobile',{version:'1.0.0'}).update.available,false);
});
test('平台冲突拒绝，运营字段不能覆盖打包样式',()=>{
 assert.equal(requestPlatform(new URL('http://localhost/?platform=desktop'),{'x-hetun-platform':'web'}),'web');
 assert.throws(()=>requestPlatform(new URL('http://localhost/?platform=mobile'),{'x-hetun-platform':'tv'}),{status:400});
 const c=operationConfig(defaultTheme);assert.equal(c.brand,undefined);assert.equal(c.colors,undefined);assert.equal(c.sections[0].style,undefined);assert.equal(c.navigation[0].icon,undefined);assert.deepEqual(Object.keys(c.carousel),['slides']);
});
test('CMS手选次序与当前页评分排序真实执行，不伪造全库热度',async t=>{
 const server=http.createServer((req,res)=>{res.setHeader('Content-Type','application/json');res.end(JSON.stringify({page:1,pagecount:2,total:4,list:[{vod_id:1,vod_name:'甲',vod_score:'6',vod_play_url:'secret-stream'},{vod_id:2,vod_name:'乙',vod_score:'9'}]}));});
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));t.after(()=>new Promise(resolve=>{server.closeAllConnections();server.close(resolve);}));const base='http://127.0.0.1:'+server.address().port;
 assert.deepEqual((await listCms(base,{ids:'2,1',orderBy:'manual'},true)).items.map(v=>v.id),['2','1']);
 const sorted=await listCms(base,{orderBy:'score'},true);assert.deepEqual(sorted.items.map(v=>v.id),['2','1']);assert.doesNotMatch(JSON.stringify(sorted),/secret-stream/);
 await assert.rejects(()=>listCms(base,{orderBy:'hot'},true),/不提供全库热度/);await assert.rejects(()=>listCms(base,{ids:'1,abc'},true),/数字/);
});
