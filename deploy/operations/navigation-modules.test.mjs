import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import {dependency} from './test-dependencies.mjs';
import {fileURLToPath} from 'node:url';
const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'../..');
const ts=dependency('typescript'),vue=dependency('vue');
const modules=new Map();
function domain(file){const full=path.join(root,'src/domain',file);if(modules.has(full))return modules.get(full);if(file.endsWith('.json'))return JSON.parse(fs.readFileSync(full,'utf8'));const out={exports:{}};modules.set(full,out.exports);const js=ts.transpileModule(fs.readFileSync(full,'utf8'),{compilerOptions:{module:ts.ModuleKind.CommonJS,esModuleInterop:true}}).outputText;new Function('require','module','exports',js)(name=>domain(name.replace('./','')+(name.endsWith('.json')?'':'.ts')),out,out.exports);return out.exports;}
const rules=domain('navigation-modules.ts'),defaults=domain('default-theme.json');
const categories=[{id:'movie',name:'电影',parentId:''},{id:'action',name:'动作',parentId:'movie'},{id:'drama',name:'电视剧',parentId:''},{id:'loop',name:'循环',parentId:'loop'}];
test('固定模块恢复保留名称、颜色、排序和关闭状态，去除旧页面绑定',()=>{
 const theme=structuredClone(defaults);const ranking=theme.navigation.find(n=>n.page==='ranking');ranking.label='我的榜单';ranking.textColor='#123456';ranking.pageId='old-page';ranking.categoryId='movie';ranking.enabled=false;
 theme.navigation=[ranking,...theme.navigation.filter(n=>n!==ranking),{id:'old-custom',page:'category',label:'旧入口',enabled:true}];
 rules.normalizeBottomNavigation(theme);assert.equal(theme.navigation.length,6);assert.deepEqual(new Set(theme.navigation.map(n=>n.page)),new Set(rules.bottomRoutes));assert.equal(theme.navigation[0].label,'我的榜单');assert.equal(theme.navigation[0].textColor,'#123456');assert.equal(theme.navigation[0].pageId,undefined);assert.equal(theme.navigation[0].categoryId,'');assert.equal(theme.features.ranking,false);
 rules.setBottomEnabled(theme,theme.navigation[0],true);rules.normalizeBottomNavigation(theme);assert.equal(theme.features.ranking,true);assert.equal(theme.navigation[0].enabled,true);
});
test('分类功能不受底部全站开关影响，未配置或关闭则不启用',()=>{
 const theme=structuredClone(defaults);theme.features.ranking=false;theme.features.schedule=false;theme.categoryShortcuts=[{entryId:'movie',items:[{id:'ranking',enabled:true},{id:'schedule',enabled:true},{id:'search',enabled:false}]}];
 assert.equal(rules.categoryRouteAllowed(theme,'movie','ranking'),true);assert.equal(rules.categoryRouteAllowed(theme,'movie','schedule'),true);assert.equal(rules.categoryRouteAllowed(theme,'drama','ranking'),false);assert.equal(rules.categoryRouteAllowed(theme,'movie','search'),false);assert.equal(rules.categoryRouteAllowed(theme,'','ranking'),false);
});
test('分类内容含子类且不混入全站内容，循环父级安全结束',()=>{
 const rows=[{id:'a',categoryId:'movie'},{id:'b',categoryId:'action'},{id:'c',categoryId:'drama'},{id:'d'},{id:'e',categoryId:'loop'}];
 assert.deepEqual(rules.scopedRows(rows,categories,'movie').map(n=>n.id),['a','b']);assert.equal(rules.scopedRows(rows,categories,''),rows);
});
test('短剧无来源时保持空白，不使用全站影视兜底',()=>{
 const theme=structuredClone(defaults);theme.navigation.find(n=>n.page==='shortdrama').categoryId='';assert.equal(rules.shortDramaSource(theme,categories),'');assert.equal(rules.shortDramaSource(theme,[...categories,{id:'short',name:'短剧',parentId:''}]),'short');theme.navigation.find(n=>n.page==='shortdrama').categoryId='manual';assert.equal(rules.shortDramaSource(theme,categories),'manual');
});
function player(api){
 const props=vue.reactive({item:undefined,bound:true,loading:false,favorite:false,ranking:false,editing:false,demo:false,blocked:false,hasNext:true,hasPrevious:false});const events=[],cleanup=[];const module={exports:{}};const scope=vue.effectScope();
 const source=fs.readFileSync(path.join(root,'src/components/MobileShortDrama.vue'),'utf8').match(/<script setup lang="ts">([\s\S]*?)<\/script>/)[1];
 const js=ts.transpileModule(source+'\nexport {url,error,pending,detail,episode,playSelected,playbackError,step,ended};',{compilerOptions:{module:ts.ModuleKind.CommonJS}}).outputText;
 scope.run(()=>new Function('require','module','exports','defineProps','defineEmits',js)(name=>name==='vue'?{...vue,getCurrentInstance:()=>null,onUnmounted:fn=>cleanup.push(fn)}:name==='@/services/api'?{api,assetUrl:x=>x}:name==='@/services/library'?{remember:()=>{}}:{},module,module.exports,()=>props,()=>((...args)=>events.push(args))));
 return {props,events,...module.exports,dispose(){cleanup.forEach(fn=>fn());scope.stop();}};
}
const film=id=>({id,title:id,sources:[{episodes:[{name:'第1集'},{name:'第2集'}]},{episodes:[{name:'第1集'},{name:'第2集'}]}]});
const settle=async()=>{await vue.nextTick();for(let i=0;i<15;i++)await Promise.resolve();};
test('短剧播放经权限接口自动重试线路，失败后可重试',async()=>{
 const calls=[];let failAll=false;const p=player({catalog:async(_p,_q,_c,id)=>({items:[film(id)]}),playback:async(id,s,e)=>{calls.push([id,s,e]);if(s===1||failAll)throw Error('unavailable');return {url:'https://media.example/'+id};}});
 p.props.item=film('a');await settle();assert.deepEqual(calls,[['a',1,1],['a',2,1]]);assert.equal(p.url.value,'https://media.example/a');
 p.playbackError();await settle();assert.equal(p.url.value,'');assert.ok(p.error.value);assert.equal(calls.length,2);await p.playSelected();assert.equal(p.url.value,'https://media.example/a');p.dispose();
});
test('快速切短剧时旧播放请求不得回写，权限关闭立即停止加载',async()=>{
 let resolveOld;const p=player({catalog:async(_p,_q,_c,id)=>({items:[film(id)]}),playback:async(id)=>id==='a'?await new Promise(r=>resolveOld=r):{url:'https://media.example/b'}});
 p.props.item=film('a');await settle();assert.equal(typeof resolveOld,'function');p.props.item=film('b');await settle();resolveOld({url:'https://media.example/a'});await settle();assert.equal(p.url.value,'https://media.example/b');
 p.props.blocked=true;await settle();assert.equal(p.url.value,'');p.dispose();
});
test('编辑预览不请求播放，滑动遵守上下边界，结束优先下一集',async()=>{
 const calls=[];const p=player({catalog:async(_p,_q,_c,id)=>({items:[film(id)]}),playback:async(...args)=>{calls.push(args);return {url:'https://media.example/a'};}});
 p.props.editing=true;p.props.item=film('a');await settle();assert.equal(calls.length,0);p.step(-1);assert.equal(p.events.length,0);p.step(1);assert.deepEqual(p.events.pop(),['step',1]);p.props.editing=false;await settle();p.ended();await settle();assert.equal(p.episode.value,1);assert.equal(calls.at(-1)[2],2);p.dispose();
});

test('短剧翻页返回前切走页面，旧翻页不得覆盖新页面索引',async()=>{
 const script=fs.readFileSync(path.join(root,'src/components/AppExperience.vue'),'utf8');
 const actual=script.match(/async function stepShort\(by:number\)\{[\s\S]*?(?=function favoriteShort)/)[0];
 const compiled=ts.transpileModule(actual,{compilerOptions:{module:ts.ModuleKind.CommonJS}}).outputText;
 const state={loading:{value:false},page:{value:'shortdrama'},shortIndex:{value:0},visible:{value:[{id:'a'}]},catalogPage:{value:1},catalogPages:{value:2}};
 const step=new Function('state','let requestGeneration=0;const {loading,page,shortIndex,visible,catalogPage,catalogPages}=state;function fetchCatalog(){requestGeneration++;return new Promise(r=>state.complete=()=>{visible.value.push({id:"b"});r();});}'+compiled+';return {stepShort,cancel(){requestGeneration++;page.value="home";shortIndex.value=0;}};')(state);
 const pending=step.stepShort(1);step.cancel();state.complete();await pending;assert.equal(state.shortIndex.value,0);
 state.page.value='shortdrama';const next=step.stepShort(1);await next;assert.equal(state.shortIndex.value,1);
});

const menuPages=domain('operation-pages.ts');
test('菜单目标切换保持入口外观且清除其他目标，旧菜单保留原目标',()=>{
 const theme=structuredClone(defaults);theme.contentPages=[{id:'summer',title:'暑期活动',kind:'campaign',enabled:true,categoryId:''}];
 const entry={id:'offer',label:'优惠活动',enabled:true,page:'schedule',categoryId:'movie',iconKey:'gift',textColor:'#123456',display:'both'};theme.serviceMenuItems=[entry];
 assert.equal(menuPages.menuTargetType(entry),'system');
 menuPages.setMenuTargetType(theme,entry,'external');entry.url='https://example.com/event?a=1#offer';assert.equal(menuPages.menuTargetType(entry),'external');assert.equal(entry.categoryId,'');assert.equal(entry.pageId,undefined);
 menuPages.setMenuTargetType(theme,entry,'page');assert.equal(entry.url,undefined);assert.equal(entry.pageId,'summer');assert.equal(entry.page,'home');
 menuPages.setMenuTargetType(theme,entry,'system');assert.equal(entry.pageId,undefined);assert.equal(entry.url,undefined);assert.equal(entry.label,'优惠活动');assert.equal(entry.textColor,'#123456');assert.equal(entry.iconKey,'gift');
 assert.equal(menuPages.menuTargetType({pageId:'summer'}),'page');
 theme.contentPages[0].enabled=false;menuPages.setMenuTargetType(theme,entry,'page');assert.equal(menuPages.entryVisible(theme,entry),false);
 menuPages.setMenuTargetType(theme,entry,'external');assert.equal(menuPages.entryVisible(theme,entry),true);
});
test('外部菜单仅打开完整 HTTP 网页，拒绝脚本、认证信息和异常地址',()=>{
 for(const value of ['https://example.com/event?a=1#offer','http://127.0.0.1:60609/activity','HTTPS://example.com/活动'])assert.ok(menuPages.menuExternalUrl(value),value);
 for(const value of ['',undefined,'javascript:alert(1)','data:text/html,hi','file:///a','//example.com','https://user:pass@example.com','https://','https://exa mple.com','https://example.com\n/x','https://example.com\\x','https://example.com:'+999999,'https://example.com/'+'a'.repeat(2048)])assert.equal(menuPages.menuExternalUrl(value),'',String(value));
});


test('中间导航按分类独立排版，组关闭后隐藏且保留入口顺序',()=>{
 const theme=structuredClone(defaults);
 theme.categoryShortcuts=[
  {entryId:'movie',enabled:true,layout:'grid',columns:2,items:[{id:'schedule',label:'电影排期',enabled:true},{id:'ranking',label:'电影榜',enabled:true},{id:'search',label:'找电影',enabled:false}]},
  {entryId:'series',enabled:true,layout:'scroll',columns:5,items:[{id:'search',label:'找剧',enabled:true}]}
 ];
 assert.deepEqual(menuPages.categoryShortcutSettings(theme,'movie'),{enabled:true,layout:'grid',columns:2});
 assert.deepEqual(menuPages.categoryShortcutSettings(theme,'series'),{enabled:true,layout:'scroll',columns:5});
 assert.deepEqual(menuPages.visibleCategoryShortcuts(theme,'movie').map(n=>n.id),['schedule','ranking']);
 const saved=structuredClone(theme.categoryShortcuts[0].items);
 theme.categoryShortcuts[0].enabled=false;
 assert.deepEqual(menuPages.visibleCategoryShortcuts(theme,'movie'),[]);
 assert.equal(rules.categoryRouteAllowed(theme,'movie','ranking'),false);
 assert.equal(rules.categoryRouteAllowed(theme,'series','search'),true);
 assert.deepEqual(theme.categoryShortcuts[0].items,saved);
 theme.categoryShortcuts[0].enabled=true;
 assert.deepEqual(menuPages.visibleCategoryShortcuts(theme,'movie').map(n=>n.id),['schedule','ranking']);
});
test('旧分类入口兼容可见性，未配置分类默认关闭，不继承其他分区',()=>{
 const theme=structuredClone(defaults);theme.categoryShortcuts=[{categoryId:'1',items:[{id:'ranking',enabled:true}]}];
 assert.deepEqual(menuPages.categoryShortcutSettings(theme,'movie'),{enabled:true,layout:'scroll',columns:4});
 assert.deepEqual(menuPages.categoryShortcutSettings(theme,'series'),{enabled:false,layout:'scroll',columns:4});
 assert.deepEqual(menuPages.visibleCategoryShortcuts(theme,'series'),[]);
 theme.categoryShortcuts[0].items[0].enabled=false;
 assert.equal(menuPages.categoryShortcutSettings(theme,'movie').enabled,false);
});

function primaryTheme(){
 const theme=structuredClone(defaults);
 theme.categories=[
  {id:'anime',label:'动漫',enabled:true,page:'category',categoryId:'movie'},
  {id:'featured',label:'精选',enabled:true,page:'category',categoryId:'movie'},
  {id:'new',label:'新活动',enabled:true,page:'home',categoryId:''}
 ];
 theme.categoryShortcuts=[
  {entryId:'anime',enabled:true,layout:'grid',columns:2,items:[{id:'all',label:'全部动漫',enabled:true},{id:'ranking',label:'动漫榜',enabled:true},{id:'schedule',label:'动漫排期',enabled:true},{id:'search',label:'找动漫',enabled:true}]},
  {entryId:'featured',enabled:false,layout:'scroll',columns:4,items:[{id:'ranking',label:'精选榜',enabled:true}]},
  {entryId:'new',enabled:true,items:[{id:'all',label:'全部活动',enabled:true}]}
 ];return theme;
}
test('APP 栏目绑定相同 CMS 资源仍独立，新栏目和未绑定栏目可单独设计',()=>{
 const theme=primaryTheme();
 assert.deepEqual(menuPages.categoryShortcutSettings(theme,'anime'),{enabled:true,layout:'grid',columns:2});
 assert.deepEqual(menuPages.visibleCategoryShortcuts(theme,'featured'),[]);
 assert.equal(menuPages.categoryShortcuts(theme,'featured')[0].label,'精选榜');
 assert.equal(menuPages.visibleCategoryShortcuts(theme,'new')[0].label,'全部活动');
 assert.equal(rules.categoryRouteAllowed(theme,'new','category'),false);
 theme.categories[0].label='新动漫名称';theme.categories[0].categoryId='drama';
 assert.equal(menuPages.visibleCategoryShortcuts(theme,'anime')[1].label,'动漫榜');
 assert.equal(menuPages.categoryShortcutSettings(theme,'featured').enabled,false);
 theme.categories.push({id:'later',label:'稍后新增',enabled:true,page:'home',categoryId:'movie'});
 assert.equal(menuPages.categoryShortcutSettings(theme,'later').enabled,false);
 assert.equal(menuPages.visibleCategoryShortcuts(theme,'later').length,0);
});
test('旧 CMS 归属只迁移到唯一 APP 栏目，多个绑定不会串用旧配置',()=>{
 const theme=primaryTheme();theme.categoryShortcuts=[{categoryId:'movie',items:[{id:'ranking',label:'旧榜',enabled:true}]}];
 assert.equal(menuPages.categoryShortcutGroup(theme,'anime'),undefined);
 assert.equal(menuPages.categoryShortcutGroup(theme,'featured'),undefined);
 theme.categories=theme.categories.filter(n=>n.id!=='featured');
 assert.equal(menuPages.visibleCategoryShortcuts(theme,'anime')[0].label,'旧榜');
 theme.categoryShortcuts.push({entryId:'anime',enabled:false,items:[]});
 assert.deepEqual(menuPages.visibleCategoryShortcuts(theme,'anime'),[]);
});
function experience(theme){
 const props=vue.reactive({theme,items:[{id:'a',categoryId:'movie'},{id:'b',categoryId:'action'},{id:'c',categoryId:'drama'}],categories,sectionItems:{},page:'home',pageId:'',platform:'mobile',editing:true,studio:true});
 const calls=[],events=[],module={exports:{}};const scope=vue.effectScope();
 const source=fs.readFileSync(path.join(root,'src/components/AppExperience.vue'),'utf8').match(/<script setup lang="ts">([\s\S]*?)<\/script>/)[1].replaceAll('import.meta.env.VITE_HETUN_APP_VERSION','"0.3.15"');
 const js=ts.transpileModule(source+'\nexport {page,scopeRoot,activePrimaryEntryId,middleNav,middleSettings,categoryId,visible,scheduleRows,scopedDiscovery,scopeCategories,message,go,navigate,middleGo,fetchCatalog,bindCategory,activePageId,sections,contentPage,isHomeContent,operationsSlides,isEntryActive,carouselAction};',{compilerOptions:{module:ts.ModuleKind.CommonJS,esModuleInterop:true}}).outputText;
 const api={catalog:async(...args)=>{calls.push(args);return {items:props.items,page:1,pages:1};}};
 scope.run(()=>new Function('require','module','exports','defineProps','withDefaults','defineEmits','defineExpose','uni',js)(name=>name==='vue'?{...vue,onMounted:()=>{},onUnmounted:()=>{}}:name.startsWith('@/domain/')?domain(name.slice('@/domain/'.length)+'.ts'):name==='@/services/api'?{api,assetUrl:x=>x,memberApi:{}}:name==='@/services/library'?{getLibrary:()=>[],getHistory:()=>[]}: {},module,module.exports,()=>props,x=>x,()=>((...args)=>events.push(args)),()=>{},{getStorageSync:()=>'',getSystemInfoSync:()=>({platform:'web'})}));
 return {props,calls,events,...module.exports,dispose:()=>scope.stop()};
}
test('实际预览按 APP 身份切换中间导航，绑定资源后功能仅查本分类及子类',async()=>{
 const theme=primaryTheme();theme.features.ranking=false;theme.features.schedule=false;
 theme.schedule=[{id:'s1',categoryId:'action',date:'2026-11-01'},{id:'s2',categoryId:'drama',date:'2026-11-02'}];
 const p=experience(theme);
 try{
  p.navigate(p.props.theme.categories[0]);await settle();
  assert.equal(p.activePrimaryEntryId.value,'anime');assert.equal(p.scopeRoot.value,'movie');assert.equal(p.middleNav.value.length,4);
  p.middleGo('ranking');await settle();assert.equal(p.page.value,'ranking');assert.equal(p.calls.at(-1)[2],'movie');assert.equal(p.calls.at(-1)[4],'score');assert.deepEqual(p.visible.value.map(n=>n.id),['a','b']);
  p.middleGo('schedule');assert.deepEqual(p.scheduleRows.value.map(n=>n.id),['s1']);
  p.navigate(p.props.theme.categories[1]);await settle();assert.equal(p.activePrimaryEntryId.value,'featured');assert.equal(p.middleNav.value.length,0);assert.equal(p.middleSettings.value.layout,'scroll');
  p.navigate(p.props.theme.categories[0]);await settle();p.props.theme.categories[0].categoryId='drama';await settle();
  assert.equal(p.activePrimaryEntryId.value,'anime');assert.equal(p.scopeRoot.value,'drama');assert.equal(p.middleSettings.value.layout,'grid');assert.deepEqual(p.visible.value.map(n=>n.id),['c']);
  p.props.theme.features.ranking=true;await settle();p.go('ranking');await settle();assert.equal(p.activePrimaryEntryId.value,'');assert.equal(p.scopeRoot.value,'');assert.deepEqual(p.visible.value.map(n=>n.id),['a','b','c']);
 }finally{p.dispose();}
});
test('未绑定栏目不发送全站请求，解除绑定清空旧资源且保留独立配置',async()=>{
 const p=experience(primaryTheme());try{
  p.navigate(p.props.theme.categories[2]);await settle();const count=p.calls.length;
  assert.equal(p.activePrimaryEntryId.value,'new');p.middleGo('all');await p.fetchCatalog();
  assert.equal(p.calls.length,count);assert.equal(p.visible.value.length,0);assert.equal(p.scopedDiscovery.value.length,0);assert.equal(p.scopeCategories.value.length,0);assert.match(p.message.value,/绑定/);
  p.navigate(p.props.theme.categories[0]);await settle();p.middleGo('ranking');await settle();p.props.theme.categories[0].categoryId='';await settle();
  assert.equal(p.scopeRoot.value,'');assert.equal(p.page.value,'category');assert.equal(p.visible.value.length,0);assert.equal(p.middleNav.value.length,4);
 }finally{p.dispose();}
});


test('旧配置首次归属固定到 APP，改绑 CMS 和删除新建均不继承旧导航',()=>{
 const theme=primaryTheme();theme.categories=theme.categories.filter(n=>n.id!=='featured');
 theme.categoryShortcuts=[{categoryId:'movie',enabled:true,layout:'grid',columns:2,items:[{id:'ranking',label:'旧榜',enabled:true}]}];
 menuPages.migrateCategoryShortcutOwners(theme);
 assert.equal(theme.categoryShortcuts[0].entryId,'anime');assert.equal(theme.categoryShortcuts[0].categoryId,undefined);
 theme.categories[0].categoryId='drama';
 assert.equal(menuPages.visibleCategoryShortcuts(theme,'anime')[0].label,'旧榜');
 theme.categories.push({id:'later',categoryId:'movie',enabled:true,page:'home',label:'新增'});
 assert.equal(menuPages.categoryShortcutSettings(theme,'later').enabled,false);
 theme.categories[0].pageId='anime-page';theme.contentPages=[{id:'anime-page',title:'动漫',kind:'category',enabled:true,categoryId:'drama'}];
 menuPages.removeContentPage(theme,'anime-page');assert.deepEqual(theme.categoryShortcuts,[]);
 assert.equal(menuPages.categoryShortcutSettings(theme,'later').enabled,false);
});
test('歧义或无主旧导航永久清理，显式 APP 配置始终优先',()=>{
 const theme=primaryTheme();theme.categoryShortcuts=[{categoryId:'movie',items:[{id:'ranking',label:'旧榜',enabled:true}]},{categoryId:'missing',items:[]}];
 menuPages.migrateCategoryShortcutOwners(theme);assert.deepEqual(theme.categoryShortcuts,[]);
 theme.categories=theme.categories.filter(n=>n.id!=='featured');assert.equal(menuPages.categoryShortcutSettings(theme,'anime').enabled,false);
 for(const reverse of [false,true]){
  theme.categoryShortcuts=[{categoryId:'movie',items:[{id:'ranking',label:'旧榜',enabled:true}]},{entryId:'anime',enabled:false,layout:'grid',columns:3,items:[]}];
  if(reverse)theme.categoryShortcuts.reverse();menuPages.migrateCategoryShortcutOwners(theme);
  assert.equal(theme.categoryShortcuts.length,1);assert.equal(theme.categoryShortcuts[0].entryId,'anime');
  assert.deepEqual(menuPages.categoryShortcutSettings(theme,'anime'),{enabled:false,layout:'grid',columns:3});
 }
});

test('删除页面先迁移旧组，保留其他存活 APP 分类的独立配置',()=>{
 const theme=primaryTheme();theme.categories=theme.categories.filter(n=>n.id!=='featured');
 theme.categories.push({id:'other',categoryId:'drama',enabled:true,page:'home',label:'剧集',pageId:'remove-me'});
 theme.contentPages=[{id:'remove-me',title:'待删除',kind:'category',enabled:true,categoryId:'drama'}];
 theme.categoryShortcuts=[{categoryId:'movie',enabled:true,items:[{id:'ranking',label:'电影榜',enabled:true}]},{categoryId:'drama',enabled:true,items:[]}];
 menuPages.removeContentPage(theme,'remove-me');
 assert.equal(theme.categoryShortcuts.length,1);assert.equal(theme.categoryShortcuts[0].entryId,'anime');
 assert.equal(menuPages.visibleCategoryShortcuts(theme,'anime')[0].label,'电影榜');
});
function shortcutEditor(theme){
 const props=vue.reactive({theme,categories}),module={exports:{}},scope=vue.effectScope();
 const source=fs.readFileSync(path.join(root,'src/components/OperationsShortcutsEditor.vue'),'utf8').match(/<script setup lang="ts">([\s\S]*?)<\/script>/)[1];
 const js=ts.transpileModule(source+'\nexport {bindingId,categoryRows,availableCategories,displayFor,addBinding,toggleCategory,setDisplay};',{compilerOptions:{module:ts.ModuleKind.CommonJS}}).outputText;
 scope.run(()=>new Function('require','module','exports','defineProps',js)(name=>name==='vue'?vue:name.startsWith('@/domain/')?domain(name.slice('@/domain/'.length)+'.ts'):{},module,module.exports,()=>props));
 return {props,...module.exports,dispose:()=>scope.stop()};
}
test('中间导航只列已绑定组，添加绑定不重复、不自动开启，也不按 CMS 资源串组',async()=>{
 const theme=primaryTheme();theme.categoryShortcuts=[];const before=JSON.stringify(theme),p=shortcutEditor(theme);
 try{
  assert.equal(JSON.stringify(p.props.theme),before);assert.equal(p.categoryRows.value.length,0);assert.equal(p.availableCategories.value.length,3);
  p.bindingId.value='anime';p.addBinding();await settle();
  assert.deepEqual(p.categoryRows.value.map(n=>n.entry.id),['anime']);
  assert.equal(p.props.theme.categoryShortcuts[0].entryId,'anime');assert.equal(p.props.theme.categoryShortcuts[0].enabled,false);
  assert.equal(p.props.theme.categoryShortcuts[0].items.length,4);assert.ok(p.props.theme.categoryShortcuts[0].items.every(n=>!n.enabled&&n.display==='both'));
  p.bindingId.value='anime';p.addBinding();p.bindingId.value='missing';p.addBinding();
  assert.equal(p.props.theme.categoryShortcuts.length,1);
  p.bindingId.value='featured';p.addBinding();await settle();p.toggleCategory('featured',true);
  assert.equal(p.props.theme.categoryShortcuts[1].enabled,true);assert.ok(p.props.theme.categoryShortcuts[1].items.every(n=>!n.enabled));
  assert.deepEqual(menuPages.visibleCategoryShortcuts(p.props.theme,'featured'),[]);
  assert.deepEqual(menuPages.visibleCategoryShortcuts(p.props.theme,'anime'),[]);
  assert.equal(p.availableCategories.value[0].id,'new');
 }finally{p.dispose();}
});
test('整组文字和图文切换持久化既有字段，保留开关、素材、顺序及其他分类',async()=>{
 const theme=primaryTheme();theme.categoryShortcuts[0].items[0].iconKey='custom-slot';const p=shortcutEditor(theme);
 try{
  const other=JSON.stringify(p.props.theme.categoryShortcuts[1]);const initial=structuredClone(theme.categoryShortcuts[0]);
  p.setDisplay('anime','text');await settle();assert.equal(p.displayFor('anime'),'text');
  assert.deepEqual(p.props.theme.categoryShortcuts[0],{...initial,items:initial.items.map(n=>({...n,display:'text'}))});
  p.toggleCategory('anime',false);p.setDisplay('anime','both');await settle();assert.equal(p.displayFor('anime'),'both');
  assert.equal(p.props.theme.categoryShortcuts[0].enabled,false);assert.equal(JSON.stringify(p.props.theme.categoryShortcuts[1]),other);
  assert.equal(p.props.theme.categoryShortcuts[0].items[0].iconKey,'custom-slot');p.toggleCategory('anime',true);
  const saved=JSON.parse(JSON.stringify(p.props.theme));assert.ok(menuPages.visibleCategoryShortcuts(saved,'anime').every(n=>n.display==='both'));
  p.setDisplay('featured','text');await settle();
  assert.ok(p.props.theme.categoryShortcuts[1].items.every(n=>n.display==='text'));
  assert.equal(p.props.theme.categoryShortcuts[1].enabled,false);
 }finally{p.dispose();}
});
test('删除与重新加载分类后列表更新，不修改已移除或未绑定的组',async()=>{
 const p=shortcutEditor(primaryTheme());try{
  assert.deepEqual(p.categoryRows.value.map(n=>n.entry.id),['anime','featured','new']);
  p.props.theme.categories.splice(1,1);await settle();assert.deepEqual(p.categoryRows.value.map(n=>n.entry.id),['anime','new']);
  const orphan=JSON.stringify(p.props.theme.categoryShortcuts[1]);p.toggleCategory('featured',true);p.setDisplay('featured','text');
  assert.equal(JSON.stringify(p.props.theme.categoryShortcuts[1]),orphan);
  p.props.theme={...primaryTheme(),categoryShortcuts:[]};await settle();assert.equal(p.categoryRows.value.length,0);
  p.toggleCategory('anime',true);p.setDisplay('anime','text');assert.deepEqual(p.props.theme.categoryShortcuts,[]);
  p.bindingId.value='new';p.addBinding();await settle();assert.deepEqual(p.categoryRows.value.map(n=>n.entry.id),['new']);
  assert.equal(menuPages.categoryShortcutSettings(p.props.theme,'new').enabled,false);
 }finally{p.dispose();}
});

test('栏目独立绑定不会随页面来源改变，改绑和解绑不影响相邻栏目',()=>{
 const theme=structuredClone(defaults);
 const page={id:'independent',title:'页面',kind:'category',categoryId:'movie',enabled:true};theme.contentPages=[page];
 theme.navigation=[{id:'nav',pageId:page.id,page:'category',categoryId:'movie',enabled:true}];
 theme.sections=[{id:'a',pageId:page.id,source:'category',categoryId:'movie',limit:6,orderBy:'score'},{id:'b',pageId:page.id,source:'category',categoryId:'drama',limit:9,orderBy:'latest'}];
 const before=structuredClone(theme.sections);
 menuPages.bindPageCategory(theme,page,'drama');assert.equal(page.categoryId,'drama');assert.equal(theme.navigation[0].categoryId,'drama');assert.deepEqual(theme.sections,before);
 menuPages.bindPageCategory(theme,page,'');assert.deepEqual(theme.sections,before);
 const restored=JSON.parse(JSON.stringify(theme));assert.deepEqual(restored.sections,before);
});
test('独立栏目读取自身一级分类及子分类，保留排序数量和空绑定边界',()=>{
 const presentation=domain('presentation.ts');
 const rows=[{id:'a',categoryId:'movie',score:'7'},{id:'b',categoryId:'action',score:'9'},{id:'c',categoryId:'drama',score:'10'}];
 const section={id:'a',pageId:'p',source:'category',categoryId:'movie',limit:1,orderBy:'score'};
 const own=menuPages.sectionRows(section,rows,categories,'drama');assert.deepEqual(own.map(v=>v.id),['a','b']);
 assert.deepEqual(presentation.moduleItems(section,own,categories).map(v=>v.id),['b']);
 section.categoryId='drama';assert.deepEqual(menuPages.sectionRows(section,rows,categories,'movie').map(v=>v.id),['c']);
 section.categoryId='';assert.deepEqual(menuPages.sectionRows(section,rows,categories,'movie'),[]);
 assert.deepEqual(menuPages.sectionRows({...section,pageId:undefined,categoryId:'movie'},rows,categories,'',true),[]);
 assert.deepEqual(menuPages.sectionRows({...section,categoryId:'movie'},rows,categories,'',true).map(v=>v.id),['a','b']);
 section.categoryId='movie';section.orderBy='manual';section.manualIds='c,b,a';section.limit=6;
 assert.deepEqual(presentation.moduleItems(section,rows,categories).map(v=>v.id),['b','a']);
});


function setupEditor(component,theme,fields,extra={}){
 const props=vue.reactive({theme,categories,loadCategories:async()=>categories,active:'',page:'home',pageId:'',...extra});
 const events=[],modals=[],module={exports:{}},scope=vue.effectScope();
 const source=fs.readFileSync(path.join(root,'src/components',component+'.vue'),'utf8').match(/<script setup lang="ts">([\s\S]*?)<\/script>/)[1];
 const js=ts.transpileModule(source+'\nexport {'+fields+'};',{compilerOptions:{module:ts.ModuleKind.CommonJS,esModuleInterop:true}}).outputText;
 scope.run(()=>new Function('require','module','exports','defineProps','defineEmits','uni',js)(name=>name==='vue'?vue:name.startsWith('@/domain/')?domain(name.slice('@/domain/'.length)+'.ts'):{},module,module.exports,()=>props,()=>((...args)=>events.push(args)),{showModal:x=>modals.push(x)}));
 return {props,events,modals,...module.exports,dispose:()=>scope.stop()};
}
test('一级分类自动建独立页面，同 CMS 不共享页面，旧首页内容仅迁移一次',()=>{
 const theme=primaryTheme();theme.contentPages=[];
 theme.sections=[{id:'legacy-home',page:'home',enabled:true},{id:'other-route',page:'ranking',enabled:true}];
 theme.carousel.slides=[{id:'old-banner',page:'home',image:'hero.jpg'}];
 assert.equal(menuPages.ensurePrimaryContentPages(theme),3);
 const [anime,featured,home]=theme.categories.map(n=>menuPages.findContentPage(theme,n.pageId));
 assert.notEqual(anime.id,featured.id);assert.equal(anime.primaryEntryId,'anime');assert.equal(anime.categoryId,'movie');
 assert.equal(home.kind,'custom');assert.equal(theme.sections[0].pageId,home.id);assert.equal(theme.sections[1].pageId,undefined);assert.equal(theme.carousel.slides[0].pageId,home.id);
 assert.equal(menuPages.ensurePrimaryContentPages(theme),0);assert.equal(theme.contentPages.length,3);assert.equal(theme.sections.length,2);
});
test('原有手工绑定、独立栏目和页面名称在自动建页及 CMS 改绑后保留',()=>{
 const theme=primaryTheme();theme.contentPages=[{id:'manual',title:'手工页面',kind:'campaign',enabled:true,categoryId:''}];theme.categories[1].pageId='manual';
 theme.sections=[{id:'own-column',pageId:'manual',categoryId:'drama',enabled:true}];
 assert.equal(menuPages.ensurePrimaryContentPages(theme),2);assert.equal(theme.categories[1].pageId,'manual');assert.equal(theme.contentPages[0].primaryEntryId,undefined);
 const entry=theme.categories[0],page=menuPages.findContentPage(theme,entry.pageId);
 theme.sections.push({id:'anime-column',pageId:page.id,categoryId:'action',enabled:true});
 menuPages.renamePrimaryEntry(theme,entry,'');menuPages.renamePrimaryEntry(theme,entry,'新动漫');assert.equal(page.title,'新动漫');
 page.title='我的独立标题';menuPages.renamePrimaryEntry(theme,entry,'再次改名');assert.equal(page.title,'我的独立标题');
 menuPages.bindPrimaryCategory(theme,entry,'drama');assert.equal(page.categoryId,'drama');assert.equal(theme.sections[1].categoryId,'action');
 page.categoryId='action';menuPages.bindPrimaryCategory(theme,entry,'movie');assert.equal(page.categoryId,'action');
 menuPages.bindPage(theme,entry,'manual');assert.equal(page.primaryEntryId,undefined);assert.equal(theme.sections.length,2);assert.equal(theme.contentPages[0].title,'手工页面');
});
test('实际导航编辑器新建即关联页面，删除保留内容，页面满额时拒绝孤立分类',()=>{
 const theme=primaryTheme();theme.contentPages=[];
 const e=setupEditor('OperationsNavigationEditor',theme,'add,remove,renameEntry,bindCategory,selectedId');
 try{
  e.add();const entry=e.props.theme.categories.at(-1),page=menuPages.findContentPage(e.props.theme,entry.pageId);
  assert.equal(page.primaryEntryId,entry.id);assert.equal(entry.enabled,false);assert.equal(e.selectedId.value,entry.id);
  e.renameEntry('测试分类');e.bindCategory(entry,'drama');assert.equal(page.title,'测试分类');assert.equal(page.categoryId,'drama');
  e.props.theme.sections.push({id:'keep',pageId:page.id,enabled:true});e.remove(entry);e.modals.pop().success({confirm:true});
  assert.ok(!e.props.theme.categories.some(n=>n.id===entry.id));assert.ok(e.props.theme.contentPages.some(p=>p.id===page.id));assert.equal(page.primaryEntryId,undefined);assert.ok(e.props.theme.sections.some(s=>s.id==='keep'));
  while(e.props.theme.contentPages.length<40)e.props.theme.contentPages.push({id:'full-'+e.props.theme.contentPages.length,title:'保留页',kind:'custom',enabled:true});
  const count=e.props.theme.categories.length;e.add();assert.equal(e.props.theme.categories.length,count);assert.equal(e.props.theme.contentPages.length,40);assert.match(e.events.at(-1)[1],/40/);
 }finally{e.dispose();}
});
test('页面编排默认选自动首页，栏目及轮播按 APP 页面独立展示',async()=>{
 const theme=primaryTheme();theme.contentPages=[];menuPages.ensurePrimaryContentPages(theme);
 theme.sections=[{id:'anime-list',pageId:theme.categories[0].pageId,enabled:true},{id:'home-list',pageId:theme.categories[2].pageId,enabled:true}];
 theme.carouselGroups=[{id:'anime-hero',entryIds:['anime'],enabled:true,slides:[]},{id:'featured-hero',entryIds:['featured'],enabled:true,slides:[]}];
 const e=setupEditor('OperationsPagesEditor',theme,'currentPageId,current,modules,boundCarousels,pageOptions,selectedPage');
 try{
  assert.equal(e.currentPageId.value,theme.categories[2].pageId);assert.deepEqual(e.modules.value.map(s=>s.id),['home-list']);assert.ok(e.pageOptions.value.every(p=>p.id!=='route:home'));
  e.props.page='category';e.props.pageId=theme.categories[0].pageId;await settle();assert.deepEqual(e.modules.value.map(s=>s.id),['anime-list']);assert.deepEqual(e.boundCarousels.value.map(g=>g.id),['anime-hero']);
  e.props.pageId=theme.categories[1].pageId;await settle();assert.equal(e.modules.value.length,0);assert.deepEqual(e.boundCarousels.value.map(g=>g.id),['featured-hero']);
 }finally{e.dispose();}
});
test('APP 首页与底部首页保持一致，分类切换使用各自页面和轮播关闭状态',async()=>{
 const theme=primaryTheme();theme.contentPages=[];menuPages.ensurePrimaryContentPages(theme);
 theme.carouselGroups=[{id:'anime-hero',entryIds:['anime'],enabled:true,slides:[{id:'a',enabled:true,target:{type:'system',page:'home'}}]},{id:'featured-hero',entryIds:['featured'],enabled:false,slides:[{id:'b',enabled:true}]}];
 theme.sections=[{id:'anime-only',pageId:theme.categories[0].pageId,enabled:true}];
 const p=experience(theme);try{
  const home=p.props.theme.navigation.find(n=>n.page==='home');assert.equal(p.activePageId.value,theme.categories[2].pageId);assert.equal(p.isHomeContent.value,true);assert.equal(p.isEntryActive(home),true);
  p.navigate(p.props.theme.categories[0]);await settle();assert.equal(p.activePageId.value,theme.categories[0].pageId);assert.deepEqual(p.sections.value.map(s=>s.id),['anime-only']);assert.equal(p.operationsSlides.value[0].id,'a');assert.equal(p.isHomeContent.value,false);
  p.navigate(p.props.theme.categories[1]);await settle();assert.deepEqual([...p.operationsSlides.value],[]);assert.equal(p.sections.value.length,0);
  p.carouselAction({type:'page',pageId:theme.categories[0].pageId});await settle();assert.equal(p.activePageId.value,theme.categories[0].pageId);
  p.navigate(home);await settle();assert.equal(p.activePageId.value,theme.categories[2].pageId);assert.equal(p.isHomeContent.value,true);assert.equal(p.isEntryActive(home),true);
 }finally{p.dispose();}
});
