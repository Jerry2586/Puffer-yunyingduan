import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {dependency} from './test-dependencies.mjs';
const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'../..');
const ts=dependency('typescript'),vue=dependency('vue');
const tick=()=>new Promise(resolve=>setImmediate(resolve));
function deferred(){let resolve,reject;const promise=new Promise((a,b)=>{resolve=a;reject=b;});return {promise,resolve,reject};}
function load(file,modules={},globals={}){
 const mod={exports:{}};
 const output=ts.transpileModule(fs.readFileSync(path.join(root,file),'utf8'),{compilerOptions:{module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2020}}).outputText;
 new Function('require','module','exports',...Object.keys(globals),output)(id=>{if(id in modules)return modules[id];throw Error('Unexpected import '+id);},mod,mod.exports,...Object.values(globals));
 return mod.exports;
}
function autosave(options){
 const dispose=[],timers=new Map();let seq=0;
 const {useSharedAutosave}=load('src/composables/useSharedAutosave.ts',{vue:{...vue,onBeforeUnmount:f=>dispose.push(f)}},{setTimeout:f=>{timers.set(++seq,f);return seq;},clearTimeout:id=>timers.delete(id),uni:{showModal:r=>r.success({confirm:true})}});
 const state=useSharedAutosave({validate:()=>'',...options});
 return {state,timers,dispose:()=>dispose.forEach(f=>f()),fire:()=>{for(const [id,f] of [...timers]){timers.delete(id);f();}}};
}
test('shared autosave keeps newer typing, serializes writes, and uses the canonical server version',async()=>{
 const calls=[],first=deferred();
 const h=autosave({get:async()=>({version:1,label:'initial'}),save:async snapshot=>{calls.push(snapshot);if(calls.length===1)return first.promise;return {...snapshot,version:3,label:snapshot.label.trim()};}});
 await h.state.load();assert.equal(h.timers.size,0);
 h.state.draft.value.label='first';const waiting=h.state.flush();
 h.state.draft.value.label=' newest ';assert.equal(calls.length,1);
 first.resolve({version:2,label:'FIRST'});assert.equal(await waiting,true);
 assert.deepEqual(calls,[{version:1,label:'first'},{version:2,label:' newest '}]);
 assert.deepEqual({...h.state.draft.value},{version:3,label:'newest'});assert.equal(h.state.dirty.value,false);
 h.fire();await tick();assert.equal(calls.length,2);h.dispose();
});
test('shared autosave halts on CAS conflict and confirmed discard cancels queued writes',async()=>{
 let calls=0;const h=autosave({get:async()=>({version:1,label:'initial'}),save:async()=>{calls++;throw Object.assign(Error('changed elsewhere'),{status:409});}});
 await h.state.load();h.state.draft.value.label='draft';assert.equal(await h.state.flush(),false);
 assert.equal(h.state.conflict.value,true);h.state.draft.value.label='still retained';h.fire();await tick();
 assert.equal(calls,1);assert.equal(h.state.draft.value.label,'still retained');
 assert.equal(await h.state.prepareToLeave(),true);h.fire();await tick();
 assert.equal(calls,1);assert.equal(h.state.dirty.value,false);assert.equal(h.state.status.value,'已自动保存');h.dispose();
});
test('shared autosave validates before write and ignores a response after disposal',async()=>{
 let calls=0;const first=deferred();const h=autosave({get:async()=>({version:1,label:'initial'}),validate:s=>s.label?'':'required',save:async()=>{calls++;return first.promise;}});
 await h.state.load();h.state.draft.value.label='';assert.equal(await h.state.flush(),false);assert.equal(calls,0);
 h.state.draft.value.label='pending';const waiting=h.state.flush();h.dispose();first.resolve({version:2,label:'late'});await waiting;
 assert.equal(h.state.saved.value.version,1);assert.equal(h.timers.size,0);
});
function storage(){const values=new Map();return {values,getStorageSync:k=>values.get(k),setStorageSync:(k,v)=>values.set(k,v),removeStorageSync:k=>values.delete(k)};}
test('history identity requires verified current token, service, CMS source and member',()=>{
 const uni=storage(),runtime={sourceKey:'cms-a'};
 const m=load('src/services/member-session.ts',{vue,'./application-runtime':{applicationRuntime:runtime}},{uni});
 uni.setStorageSync('hetun-member-token','token-a');uni.setStorageSync('hetun-member-identity',{id:'7',service:'local',source:'cms-a'});
 assert.equal(m.historyMember('local'),false);m.receiveMember({user_id:7},'local');assert.equal(m.historyMember('local'),true);
 assert.equal(uni.getStorageSync('hetun-member-identity'),undefined);
 const scope=m.historyScope('local');uni.setStorageSync('hetun-member-token','token-b');assert.equal(m.historyMember('local'),false);assert.notEqual(m.historyScope('local'),scope);
 uni.setStorageSync('hetun-member-token','token-a');runtime.sourceKey='cms-b';assert.equal(m.historyMember('local'),false);
 runtime.sourceKey='cms-a';assert.equal(m.historyMember('other'),false);
});
function library(api={}){
 const uni=storage(),runtime={history:{enabled:true,intervalSeconds:60,limit:100}},owner={scope:'member-a',member:true};
 const calls=[];const applicationApi={remember:async(...args)=>{calls.push(args);},history:async()=>({items:[]}),...api};
 const lib=load('src/services/library.ts',{'./application-runtime':{applicationRuntime:runtime},'./application':{applicationApi},'./api':{getServiceBase:()=> 'local'},'./member-session':{historyScope:()=>owner.scope,historyMember:()=>owner.member}},{uni});
 return {lib,owner,uni,runtime,calls};
}
const film=id=>({id:String(id),title:'Film '+id,poster:'',sources:[]});
test('history normalizes numeric input and never writes an old playback session into the next account',async()=>{
 const h=library();h.lib.remember(film(1),2.9,9.9,true);await tick();
 assert.equal(h.lib.getHistory()[0].historyEpisode,2);assert.equal(h.lib.getHistory()[0].historyPosition,9);assert.deepEqual(h.calls,[["1",2,9]]);
 h.owner.scope='member-b';h.lib.remember(film(1),3,15,true,'member-a');assert.equal(h.lib.getHistory().length,0);
 h.lib.remember(film(2),Infinity,NaN,true);await tick();assert.equal(h.lib.getHistory()[0].historyEpisode,1);assert.equal(h.lib.getHistory()[0].historyPosition,0);
 h.owner.scope='member-a';assert.equal(h.lib.getHistory()[0].historyPosition,9);
});
test('cloud history ignores late responses from another account and merges latest local progress',async()=>{
 const response=deferred(),h=library({history:()=>response.promise});h.lib.remember(film(1),1,25,true);await tick();
 const pending=h.lib.syncHistory();h.owner.scope='member-b';response.resolve({items:[{videoId:'1',episode:1,position:500,updatedAt:'2099-01-01T00:00:00Z'}]});await pending;assert.equal(h.lib.getHistory().length,0);
 h.owner.scope='member-a';assert.equal(h.lib.getHistory()[0].historyPosition,25);
 const merged=h.lib.mergeHistory([{...film(1),historyPosition:30,historyUpdatedAt:'2026-11-04T12:00:00Z'}],[{videoId:'1',episode:1,position:10,updatedAt:'2026-11-04T11:00:00Z'},{videoId:'2',episode:2,position:20,updatedAt:'2026-11-04T13:00:00Z'}]);
 assert.equal(merged[0].id,'2');assert.equal(merged[1].historyPosition,30);
});
test('cloud history bounds pending writes and drops queued data after account change',async()=>{
 const first=deferred(),calls=[];const h=library({remember:async(...args)=>{calls.push(args);if(calls.length===1)return first.promise;}});
 for(let i=0;i<125;i++)h.lib.remember(film(i),1,i,true);assert.equal(calls.length,1);
 first.resolve();await tick();assert.equal(calls.length,101);assert.equal(calls.at(-1)[0],'124');
 const second=deferred(),sent=[];const next=library({remember:async(...args)=>{sent.push(args);return second.promise;}});
 next.lib.remember(film(1),1,10,true);next.lib.remember(film(2),1,20,true);next.owner.scope='member-b';second.resolve();await tick();assert.equal(sent.length,1);
});
function playback(history=[]){
 const written=[],stops=[],unmount=[];
 const {usePlaybackSession}=load('src/composables/usePlaybackSession.ts',{vue:{...vue,onBeforeUnmount:f=>unmount.push(f)},'@/services/library':{remember:(...a)=>written.push(a),getHistory:()=>history,getHistoryScope:()=> 'owner'} });
 return {p:usePlaybackSession(s=>stops.push(s)),written,stops,dispose:()=>unmount.forEach(f=>f())};
}
test('playback resumes matching episode, records only actual play, and preserves completion at zero',()=>{
 const h=playback([{id:'1',historyEpisode:2,historyPosition:45}]);h.p.start(film(1),2);assert.equal(h.p.initialTime.value,45);
 h.p.timeupdate({detail:{currentTime:46}});h.p.flush();assert.equal(h.written.length,0);
 h.p.played();h.p.timeupdate({detail:{currentTime:50}});assert.equal(h.p.completed(),true);h.dispose();assert.equal(h.written.at(-1)[2],0);
 h.p.start(film(1),1);assert.equal(h.p.initialTime.value,0);
});
test('trial clamps seeks, stops once, and restarts expired saved progress from zero',()=>{
 const h=playback([{id:'1',historyEpisode:1,historyPosition:50}]);h.p.start(film(1),1,30);assert.equal(h.p.initialTime.value,0);
 h.p.played();h.p.timeupdate({detail:{currentTime:200}});assert.equal(h.p.position.value,30);assert.equal(h.p.expired.value,true);assert.equal(h.stops.length,1);
 h.p.timeupdate({detail:{currentTime:300}});assert.equal(h.stops.length,1);assert.equal(h.p.completed(),false);assert.equal(h.written.at(-1)[2],30);
});
test('source identity, resume bounds and danmu timing survive filtering and seeks',()=>{
 const d=load('src/domain/playback-session.ts');const f={sources:[{sid:5,episodes:[1,2]},{sid:9,episodes:[1]}],id:'1'};
 assert.equal(d.playbackSourceId(f,0),5);assert.equal(d.playbackSourceId(f,1),9);assert.equal(d.resumeEpisode(f,[{id:'1',historyEpisode:2}]),1);assert.equal(d.resumeEpisode(f,[{id:'1',historyEpisode:2.5}]),0);
 assert.equal(d.danmuMilliseconds(1.234),1234);assert.equal(d.danmuSeconds(1234),1.234);
 const line=d.createDanmuTimeline();line.set([{text:'late',time:8},{text:'first',time:0},{text:'second',time:1}]);line.reset();assert.deepEqual(line.advance(0).map(v=>v.text),['first']);assert.deepEqual(line.advance(1).map(v=>v.text),['second']);assert.deepEqual(line.advance(8),[]);assert.deepEqual(line.advance(0),[]);assert.deepEqual(line.advance(1).map(v=>v.text),['second']);
});

function studioLogout({page='application',leave=async()=>true,flush=async()=>{}}={}){
 const source=fs.readFileSync(path.join(root,'src/pages/studio/index.vue'),'utf8').split('<script setup lang="ts">')[1].split('</script>')[0];
 const ast=ts.createSourceFile('studio.ts',source,ts.ScriptTarget.Latest,true);
 const declaration=ast.statements.find(node=>ts.isFunctionDeclaration(node)&&node.name?.text==='logout');assert.ok(declaration);
 const busy=vue.ref(false),authenticated=vue.ref(true),userCenterOpen=vue.ref(true),error=vue.ref(''),calls=[];
 const scope={busy,authenticated,userCenterOpen,error,operationsCenter:true,schedulePage:vue.ref(page==='schedule'),scheduleEditor:vue.ref({prepareToLeave:leave}),announcementPage:vue.ref(page==='announcement'),resolverPage:vue.ref(page==='playback'),applicationPage:vue.ref(page==='application'),announcementEditor:vue.ref({prepareToLeave:leave}),playbackEditor:vue.ref({prepareToLeave:leave}),applicationEditor:vue.ref({prepareToLeave:leave}),autosave:{flush},api:{logout:async()=>calls.push('logout')},cache:{clear:()=>calls.push('clear')}};
 const output=ts.transpileModule(declaration.getText(ast),{compilerOptions:{target:ts.ScriptTarget.ES2020}}).outputText;
 const logout=new Function(...Object.keys(scope),output+';return logout;')(...Object.values(scope));
 return {logout,busy,authenticated,userCenterOpen,error,calls};
}
test('studio logout waits for shared settings to save before ending the session',async()=>{
 for(const page of ['application','schedule']){
 const saved=deferred();let requested=0;const h=studioLogout({page,leave:async()=>{requested++;return saved.promise;}});
 const pending=h.logout();assert.equal(requested,1);assert.equal(h.busy.value,true);assert.deepEqual(h.calls,[]);assert.equal(h.authenticated.value,true);
 await h.logout();assert.equal(requested,1);saved.resolve(true);await pending;
 assert.deepEqual(h.calls,['logout','clear']);assert.equal(h.authenticated.value,false);assert.equal(h.userCenterOpen.value,false);assert.equal(h.busy.value,false);
 }
});
test('studio logout preserves the session when an editor cancels leaving',async()=>{
 for(const page of ['application','playback','announcement','schedule']){
  const h=studioLogout({page,leave:async()=>false});await h.logout();assert.deepEqual(h.calls,[]);assert.equal(h.authenticated.value,true);assert.equal(h.userCenterOpen.value,true);assert.equal(h.busy.value,false);assert.equal(h.error.value,'');
 }
});
test('studio logout retains authentication when flushing existing client settings fails',async()=>{
 const h=studioLogout({page:'other',flush:async()=>{throw Error('save failed');}});await h.logout();assert.deepEqual(h.calls,[]);assert.equal(h.authenticated.value,true);assert.equal(h.error.value,'save failed');assert.equal(h.busy.value,false);
});
