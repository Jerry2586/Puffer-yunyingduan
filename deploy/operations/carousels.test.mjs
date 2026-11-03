import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import {dependency} from './test-dependencies.mjs';
import {fileURLToPath} from 'node:url';
const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'../..');
const ts=dependency('typescript'),vue=dependency('vue'),modules=new Map();
function domain(file){
 const full=path.join(root,'src/domain',file);if(modules.has(full))return modules.get(full);
 if(file.endsWith('.json'))return JSON.parse(fs.readFileSync(full,'utf8'));
 const module={exports:{}};modules.set(full,module.exports);
 const js=ts.transpileModule(fs.readFileSync(full,'utf8'),{compilerOptions:{module:ts.ModuleKind.CommonJS,esModuleInterop:true}}).outputText;
 new Function('require','module','exports',js)(name=>domain(name.replace('./','')+(name.endsWith('.json')?'':'.ts')),module,module.exports);return module.exports;
}
const rules=domain('operation-carousels.ts'),pages=domain('operation-pages.ts');
const fixture=()=>({...structuredClone(domain('default-theme.json')),categories:[{id:'movie',label:'电影',enabled:true,categoryId:'1',page:'home'}],contentPages:[{id:'event',title:'活动',enabled:true,kind:'campaign',categoryId:''}],carouselGroups:[{id:'g',name:'电影',enabled:true,entryIds:['movie'],slides:[{id:'s',title:'展示影片',image:'https://example.com/hero.jpg',sourceVodId:'12',enabled:true,target:{type:'vod',vodId:'34'}}]}]});
function editor(api={}){
 const props=vue.reactive({theme:fixture(),categories:[{id:'1',name:'电影'}],loadCategories:async()=>[]}),cleanup=[],events=[],modals=[];
 const module={exports:{}},scope=vue.effectScope();
 const source=fs.readFileSync(path.join(root,'src/components/OperationsCarouselEditor.vue'),'utf8').match(/<script setup lang="ts">([\s\S]*?)<\/script>/)[1];
 const names='openFilms,chooseFilm,upload,removeSlide,removeGroup,searchFilms,films,picker,addGroup,addSlide,bind,toggleGroup,expandedId,bindingId,bindingGroup,nameDrafts,groupName,toggleExpanded,bindingLabel,switchRevision,toggleSlide,editingNameId,openName,finishName,rename,setTitle,setImage,setExternal,move';
 const js=ts.transpileModule(source+'\nexport {'+names+'};',{compilerOptions:{module:ts.ModuleKind.CommonJS}}).outputText;
 scope.run(()=>new Function('require','module','exports','defineProps','defineEmits','uni',js)(name=>name==='vue'?{...vue,onUnmounted:fn=>cleanup.push(fn)}:name.startsWith('@/domain/')?domain(name.slice('@/domain/'.length)+'.ts'):name==='@/services/api'?{api:{catalog:async()=>({items:[],page:1,pages:1,total:0}),...api},assetUrl:x=>x,chooseLocalFile:async()=>({name:'image.png',data:'abc'})}:{},module,module.exports,()=>props,()=>((...args)=>events.push(args)),{showModal:x=>modals.push(x)}));
 return {props,events,modals,...module.exports,dispose(){cleanup.forEach(fn=>fn());scope.stop();}};
}
const film={id:'56',title:'CMS影片',carouselImage:'javascript:bad',poster:'https://example.com/poster.jpg'};
const rehydrate=e=>{e.props.theme=JSON.parse(JSON.stringify(e.props.theme));};
const record=e=>e.props.theme.carouselGroups[0];
test('展示影片和跳转影片独立，自动保存替换主题后选择仍作用于当前条目',async()=>{
 const e=editor();try{
  const old=record(e);await e.openFilms(old,old.slides[0],'display');rehydrate(e);e.chooseFilm(film);
  const s=record(e).slides[0];assert.equal(s.image,film.poster);assert.equal(s.sourceVodId,'56');assert.equal(s.target.vodId,'34');assert.equal(old.slides[0].sourceVodId,'12');
  await e.openFilms(record(e),s,'target');rehydrate(e);e.chooseFilm({...film,id:'78'});assert.equal(record(e).slides[0].target.vodId,'78');assert.equal(record(e).slides[0].sourceVodId,'56');
 }finally{e.dispose();}
});
test('上传和删除确认期间自动保存重建对象仍按编号更新，已删除条目不会复活',async()=>{
 let finish;const e=editor({asset:()=>new Promise(r=>finish=r)});try{
  let g=record(e);const pending=e.upload(g,g.slides[0]);await Promise.resolve();rehydrate(e);finish({url:'/api/assets/'+'a'.repeat(64)+'.png'});await pending;
  assert.equal(record(e).slides[0].sourceVodId,'');assert.match(record(e).slides[0].image,/^\/api\/assets\//);
  g=record(e);e.removeSlide(g,g.slides[0]);rehydrate(e);e.modals.pop().success({confirm:true});assert.equal(record(e).slides.length,0);assert.equal(record(e).enabled,true);
  e.removeGroup(record(e));rehydrate(e);e.modals.pop().success({confirm:true});assert.equal(e.props.theme.carouselGroups.length,0);
 }finally{e.dispose();}
 const other=editor();try{const g=record(other);await other.openFilms(g,g.slides[0],'display');g.slides=[];other.chooseFilm(film);assert.equal(g.slides.length,0);}finally{other.dispose();}
});
test('切换客户端卸载后在途上传不写入，旧影片请求不覆盖新查询',async()=>{
 let finish;const e=editor({asset:()=>new Promise(r=>finish=r)});const g=record(e);const pending=e.upload(g,g.slides[0]);await Promise.resolve();e.dispose();finish({url:'https://example.com/new.png'});await pending;assert.equal(g.slides[0].sourceVodId,'12');
 const requests=[];const x=editor({catalog:()=>new Promise(r=>requests.push(r))});try{const g=record(x);await x.openFilms(g,g.slides[0],'display');const latest=x.searchFilms(2);requests[1]({items:[film],page:2,pages:2,total:2});await latest;requests[0]({items:[{...film,id:'old'}],page:1,pages:2,total:2});await Promise.resolve();assert.equal(x.films.value[0].id,'56');}finally{x.dispose();}
});
test('删除内容页仅关闭失效条目，删除分类才关闭失去绑定的轮播组',()=>{
 const theme=fixture(),g=theme.carouselGroups[0];g.slides[0].target={type:'page',pageId:'event'};pages.removeContentPage(theme,'event');assert.equal(g.slides[0].enabled,false);assert.equal(g.slides[0].target.pageId,'');assert.equal(g.enabled,true);
 theme.categories=[];pages.reconcileCarouselReferences(theme);assert.deepEqual(g.entryIds,[]);assert.equal(g.enabled,false);assert.ok(rules.carouselGroupIssue(theme,g));
 assert.equal(rules.carouselFilmImage(film),film.poster);assert.equal(rules.carouselImageValid('data:image/png,abc'),false);
});
test('新增从关闭状态开始，一个分类不能重复绑定到两组',()=>{
 const e=editor();try{e.addGroup();const fresh=e.props.theme.carouselGroups[1];assert.equal(fresh.enabled,false);e.addSlide(fresh);assert.equal(fresh.slides[0].enabled,false);e.bind(fresh,'movie');assert.deepEqual([...fresh.entryIds],[]);e.toggleGroup(fresh,true);assert.equal(fresh.enabled,false);}finally{e.dispose();}
});

function inputVNode(e,field,extra={}){
 const source=fs.readFileSync(path.join(root,'src/components/OperationsCarouselEditor.vue'),'utf8');
 const input=[...source.matchAll(/<input\b[^>]*\/>/g)].map(m=>m[0]).find(tag=>tag.includes(':value="'+field+'"'));assert.ok(input,'input exists: '+field);
 const compiled=dependency('@vue/compiler-dom').compile(input.replace(/\s+v-(?:else-if|if)="[^"]*"|\s+v-else\b/g,''),{mode:'function',prefixIdentifiers:true,expressionPlugins:['typescript']}).code;
 const js=ts.transpileModule(compiled,{compilerOptions:{target:ts.ScriptTarget.ES2020}}).outputText;
 return new Function('Vue',js)(vue)(vue.proxyRefs({...e,index:0,...extra}),[]);
}
test('名称输入事件立即同步列表，保存回填保留编辑草稿，失焦后归整并可重新加载',async()=>{
 const e=editor();try{
  e.openName('g');await vue.nextTick();const stale=record(e),node=inputVNode(e,'groupName(group)',{group:stale});assert.equal(node.props.onChange,undefined);
  node.props.onInput({detail:{value:'首页 '}});await vue.nextTick();
  assert.equal(record(e).name,'首页');assert.equal(e.groupName(record(e)),'首页 ');
  rehydrate(e);await vue.nextTick();node.props.onInput({detail:{value:'首页 活动'}});await vue.nextTick();
  assert.equal(record(e).name,'首页 活动');assert.equal(stale.name,'首页');
  node.props.onBlur({detail:{value:' 首页 活动 '}});await vue.nextTick();assert.equal(e.groupName(record(e)),'首页 活动');
  rehydrate(e);await vue.nextTick();assert.equal(record(e).name,'首页 活动');assert.equal(e.groupName(record(e)),'首页 活动');
  node.props.onInput({detail:{value:''}});await vue.nextTick();assert.equal(record(e).name,'首页 活动');assert.equal(e.groupName(record(e)),'');
  node.props.onBlur({detail:{value:''}});assert.equal(e.groupName(record(e)),'首页 活动');assert.ok(e.events.some(x=>x[1].includes('名称不能为空')));
  node.props.onInput({detail:{value:'𠮷'.repeat(80)}});assert.equal(record(e).name,'首页 活动');
  e.props.theme.carouselGroups=[];e.rename('g','已删除');assert.equal(e.props.theme.carouselGroups.length,0);
 }finally{e.dispose();}
});
test('新建模块自动展开，折叠与绑定弹窗不改变已保存配置，回填保留展开位置',async()=>{
 const e=editor();try{
  await vue.nextTick();assert.equal(e.expandedId.value,'');e.addGroup();await vue.nextTick();const fresh=e.props.theme.carouselGroups[1],id=fresh.id;
  assert.equal(e.expandedId.value,id);e.rename(id,'新建首页 ');rehydrate(e);await vue.nextTick();
  assert.equal(e.expandedId.value,id);assert.equal(e.groupName(e.props.theme.carouselGroups[1]),'新建首页 ');
  const before=JSON.stringify(e.props.theme);e.toggleExpanded(id);assert.equal(e.expandedId.value,'');e.bindingId.value=id;
  assert.equal(e.bindingGroup.value.id,id);assert.equal(JSON.stringify(e.props.theme),before);
  e.toggleExpanded(id);e.props.theme.carouselGroups=e.props.theme.carouselGroups.filter(g=>g.id!==id);await vue.nextTick();
  assert.equal(e.expandedId.value,'');assert.equal(e.bindingId.value,'');assert.equal(e.nameDrafts.value[id],undefined);
  assert.equal(record(e).name,'电影');
 }finally{e.dispose();}
});
test('各模块独立改名，开启校验失败会复位滑动开关，过期记录不修改旧对象',async()=>{
 const e=editor();try{
  e.rename('g','电影 ');e.addGroup();const second=e.props.theme.carouselGroups[1];e.rename(second.id,'首页 ');
  rehydrate(e);await vue.nextTick();assert.equal(e.groupName(record(e)),'电影 ');assert.equal(e.groupName(e.props.theme.carouselGroups[1]),'首页 ');
  e.rename('g','电影 ',true);assert.equal(e.groupName(record(e)),'电影');assert.equal(e.groupName(e.props.theme.carouselGroups[1]),'首页 ');
  const stale=record(e);rehydrate(e);e.toggleGroup(stale,false);assert.equal(record(e).enabled,false);assert.equal(stale.enabled,true);
  const revision=e.switchRevision.value;e.toggleGroup(second,true);assert.equal(e.props.theme.carouselGroups[1].enabled,false);assert.ok(e.switchRevision.value>revision);
  e.toggleSlide(stale,stale.slides[0],false);assert.equal(record(e).slides[0].enabled,false);assert.equal(stale.slides[0].enabled,true);
 }finally{e.dispose();}
});
test('每组仅一个分类，替换与解除均原子更新，占用、缺失和过期对象不可越界',async()=>{
 const e=editor();try{
  e.props.theme.categories.push({id:'anime',label:'动漫',enabled:true,categoryId:'2',page:'home'});
  const old=record(e);rehydrate(e);e.bind(old,'anime');assert.deepEqual([...record(e).entryIds],['anime']);assert.deepEqual([...old.entryIds],['movie']);
  e.addGroup();const second=e.props.theme.carouselGroups[1];e.bind(second,'movie');assert.deepEqual([...second.entryIds],['movie']);
  e.bind(record(e),'movie');assert.deepEqual([...record(e).entryIds],['anime']);
  e.bind(record(e),'missing');assert.deepEqual([...record(e).entryIds],['anime']);
  e.bind(record(e),'anime');assert.deepEqual([...record(e).entryIds],[]);assert.equal(record(e).enabled,false);
  e.bind(record(e),'anime');assert.equal(record(e).enabled,false);
  record(e).entryIds=['movie','anime'];assert.ok(rules.carouselGroupIssue(e.props.theme,record(e)));
  record(e).entryIds=['movie'];assert.ok(rules.carouselGroupIssue(e.props.theme,record(e)));
  record(e).entryIds=['missing'];assert.ok(rules.carouselGroupIssue(e.props.theme,record(e)));
 }finally{e.dispose();}
});
test('标题和图片、外链使用 Uni 支持的事件保存',async()=>{
 const e=editor();try{
  await vue.nextTick();const slide=record(e).slides[0];
  inputVNode(e,'slide.title',{slide}).props.onInput({detail:{value:'新标题'}});assert.equal(slide.title,'新标题');
  inputVNode(e,'slide.image',{slide}).props.onBlur({detail:{value:'https://example.com/updated.jpg'}});assert.equal(slide.image,'https://example.com/updated.jpg');assert.equal(slide.sourceVodId,'');
  slide.target={type:'external',url:''};inputVNode(e,'slide.target.url',{slide}).props.onBlur({detail:{value:'https://example.com/event'}});assert.equal(slide.target.url,'https://example.com/event');
 }finally{e.dispose();}
});

test('小笔编辑保留自动保存，回车或外部收起，删除模块清理编辑状态',async()=>{
 const e=editor();try{
  e.openName('g');assert.equal(e.editingNameId.value,'g');
  const node=inputVNode(e,'groupName(group)',{group:record(e)});
  node.props.onInput({detail:{value:'首页活动 '}});rehydrate(e);await vue.nextTick();
  assert.equal(e.editingNameId.value,'g');assert.equal(record(e).name,'首页活动');assert.equal(e.groupName(record(e)),'首页活动 ');
  node.props.onConfirm();assert.equal(e.editingNameId.value,'');assert.equal(e.groupName(record(e)),'首页活动');
  e.openName('g');e.rename('g','');e.finishName('g');assert.equal(e.editingNameId.value,'');assert.equal(record(e).name,'首页活动');
  e.openName('g');e.props.theme.carouselGroups=[];await vue.nextTick();assert.equal(e.editingNameId.value,'');
 }finally{e.dispose();}
});

test('已绑定轮播组空内容和全关闭条目均能开启，最后一条停用和删除不关闭总开关',()=>{
 const e=editor();try{
  let g=record(e);g.enabled=false;g.slides=[];e.toggleGroup(g,true);assert.equal(g.enabled,true);assert.equal(rules.carouselGroupIssue(e.props.theme,g),'');
  rehydrate(e);g=record(e);e.addSlide(g);assert.equal(g.enabled,true);assert.equal(g.slides[0].enabled,false);
  e.toggleGroup(g,false);e.toggleGroup(g,true);assert.equal(g.enabled,true);assert.equal(e.events.length,0);
  const slide=g.slides[0];slide.enabled=true;slide.image='https://example.com/hero.jpg';slide.target={type:'vod',vodId:'12'};
  e.toggleSlide(g,slide,false);assert.equal(g.enabled,true);
  e.removeSlide(g,slide);e.modals.pop().success({confirm:true});assert.equal(g.slides.length,0);assert.equal(g.enabled,true);
  e.toggleGroup(g,false);assert.equal(g.enabled,false);
 }finally{e.dispose();}
});

test('表格条目排序、隐藏和删除独立，自动保存回填后保留内容及总开关',async()=>{
 const e=editor();try{
  let g=record(e);e.addSlide(g);const second=g.slides[1];second.title='第二张';second.image='https://example.com/second.jpg';second.target={type:'vod',vodId:'88'};
  const secondId=second.id,firstId=g.slides[0].id;const secondContent=JSON.stringify(second);
  e.move(g,1,-1);assert.deepEqual(g.slides.map(s=>s.id),[secondId,firstId]);
  e.toggleSlide(g,g.slides[1],false);assert.equal(g.slides[0].enabled,false);assert.equal(g.enabled,true);
  assert.equal(JSON.stringify(g.slides[0]),secondContent);
  rehydrate(e);await vue.nextTick();g=record(e);e.toggleSlide(g,g.slides[0],true);assert.equal(g.slides[0].enabled,true);assert.equal(g.slides[1].enabled,false);
  e.toggleSlide(g,g.slides[0],false);assert.equal(JSON.stringify(g.slides[0]),secondContent);
  e.removeSlide(g,g.slides[0]);rehydrate(e);e.modals.pop().success({confirm:true});g=record(e);
  assert.deepEqual(g.slides.map(s=>s.id),[firstId]);assert.equal(g.slides[0].title,'展示影片');assert.equal(g.enabled,true);
 }finally{e.dispose();}
});


function carouselRuntime(theme,operationsSlides,api){
 const props=vue.reactive({theme,items:[],editing:true,operationsSlides});const events=[],cleanup=[],module={exports:{}},scope=vue.effectScope();
 const source=fs.readFileSync(path.join(root,'src/components/ThemeCarousel.vue'),'utf8').match(/<script setup lang="ts">([\s\S]*?)<\/script>/)[1];
 const js=ts.transpileModule(source+'\nexport {rows,open};',{compilerOptions:{module:ts.ModuleKind.CommonJS,esModuleInterop:true}}).outputText;
 scope.run(()=>new Function('require','module','exports','defineProps','defineEmits',js)(name=>name==='vue'?{...vue,onUnmounted:fn=>cleanup.push(fn)}:name==='@/services/api'?{api,assetUrl:x=>x}:{},module,module.exports,()=>props,()=>((...args)=>events.push(args))));
 return {props,events,...module.exports,dispose:()=>{cleanup.forEach(fn=>fn());scope.stop();}};
}
test('分类轮播展示源与点击目标分离，隐藏条目不请求，空组不回退旧轮播',async()=>{
 const theme=fixture(),calls=[];theme.carousel.slides=[{id:'legacy',title:'旧图',page:'home',vodId:'old'}];
 const slides=[{id:'display',title:'',image:'',sourceVodId:'12',enabled:true,target:{type:'vod',vodId:'34'}},{id:'hidden',sourceVodId:'99',enabled:false,target:{type:'vod',vodId:'99'}}];
 const e=carouselRuntime(theme,slides,{catalog:async(...args)=>{calls.push(args);return {items:[{id:'12',title:'展示影片',poster:'display.jpg'}]};}});
 try{
  await vue.nextTick();await Promise.resolve();assert.equal(calls[0][3],'12');assert.equal(e.rows.value.length,1);assert.equal(e.rows.value[0].title,'展示影片');assert.equal(e.rows.value[0].image,'display.jpg');
  e.props.editing=false;e.open(e.rows.value[0]);assert.equal(e.events.at(-1)[0],'action');assert.equal(e.events.at(-1)[1].vodId,'34');
  e.props.operationsSlides=[];await vue.nextTick();assert.equal(e.rows.value.length,0);
  e.props.operationsSlides=undefined;await vue.nextTick();assert.equal(e.rows.value[0].id,'legacy');
 }finally{e.dispose();}
});
test('轮播分类切换后旧影片请求不能覆盖新页面展示内容',async()=>{
 const theme=fixture(),requests=[];const slide=id=>({id:'banner-'+id,title:'',image:'',sourceVodId:id,enabled:true,target:{type:'vod',vodId:id}});
 const e=carouselRuntime(theme,[slide('12')],{catalog:()=>new Promise(resolve=>requests.push(resolve))});
 try{
  e.props.operationsSlides=[slide('34')];await vue.nextTick();requests[1]({items:[{id:'34',title:'新页面影片',poster:'new.jpg'}]});await Promise.resolve();
  requests[0]({items:[{id:'12',title:'旧页面影片',poster:'old.jpg'}]});await Promise.resolve();assert.equal(e.rows.value[0].title,'新页面影片');assert.equal(e.rows.value[0].image,'new.jpg');
 }finally{e.dispose();}
});
