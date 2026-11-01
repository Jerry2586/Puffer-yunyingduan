<script setup lang="ts">
import {computed,ref,watch} from 'vue';
import type {Theme,Category,ContentPage,AppPage,NavItem,Section} from '@/domain/types';
import {bindPage,bindPageCategory,findContentPage,pageKinds,pageRoute,removeContentPage} from '@/domain/operation-pages';
import OperationsNavigationEditor from './OperationsNavigationEditor.vue';
import CmsCategoryBinding from './CmsCategoryBinding.vue';
import EditorChoice from './EditorChoice.vue';
import OperationsLayoutFields from './OperationsLayoutFields.vue';
const props=defineProps<{theme:Theme;active:string;page:AppPage;pageId:string;categories:Category[];loadCategories:()=>Promise<Category[]>}>();
const emit=defineEmits<{preview:[page:AppPage,pageId:string];message:[message:string];edit:[]}>();
const pages=computed(()=>props.theme.contentPages||[]);
const current=computed(()=>findContentPage(props.theme,props.pageId));
const modules=computed(()=>props.theme.sections.filter(s=>props.pageId?s.pageId===props.pageId:!s.pageId&&(s.page||'home')===props.page));
const boundCarousels=computed(()=>{const ids=props.theme.categories.filter(e=>props.pageId?e.pageId===props.pageId:!e.pageId&&e.page===props.page).map(e=>e.id);return (props.theme.carouselGroups||[]).filter(g=>g.entryIds.some(id=>ids.includes(id)));});
const pageOptions=computed(()=>[{id:'route:home',label:'系统 · 首页'},...pages.value.map(p=>({id:'content:'+p.id,label:p.title||'未命名页面'}))]);
const selectedPage=computed(()=>props.pageId?'content:'+props.pageId:'route:'+props.page);
const selectedModuleId=ref(''),tab=ref<'sections'|'carousel'>('sections'),settingsOpen=ref(false);
const selectedModule=computed(()=>modules.value.find(s=>s.id===selectedModuleId.value));
const editable=computed(()=>Boolean(current.value)||props.page==='home');
watch(()=>[props.pageId,props.page,modules.value.map(s=>s.id).join(',')],()=>{
 if(!selectedModule.value)selectedModuleId.value=modules.value[0]?.id||'';
},{immediate:true});
function uid(prefix:string){return prefix+'-'+Date.now().toString(36)+'-'+Math.random().toString(36).slice(2,8);}
function preview(value:string|number){const id=String(value);if(id.startsWith('content:')){const p=findContentPage(props.theme,id.slice(8));if(p)emit('preview',pageRoute(p),p.id);}else emit('preview','home','');}
function edit(page:AppPage,id:string){emit('preview',page,id);emit('edit');}
function addPage(entry?:NavItem){
 if(pages.value.length>=40){emit('message','独立页面最多40个。');return;}
 const p:ContentPage={id:uid('page'),title:entry?.label||'新建页面',kind:'category',categoryId:entry?.categoryId||'',enabled:true,carouselEnabled:true};
 (props.theme.contentPages||(props.theme.contentPages=[])).push(p);
 if(entry)bindPage(props.theme,entry,p.id);
 createModule(p);settingsOpen.value=true;edit(pageRoute(p),p.id);
}
function changeKind(value:string|number){
 const p=current.value;if(!p)return;const kind=String(value) as ContentPage['kind'];
 if(kind==='blank'&&(modules.value.length||boundCarousels.value.length||props.theme.carousel.slides.some(s=>s.pageId===p.id))){emit('message','请先移除本页栏目和轮播绑定，再设为空白页。');return;}
 p.kind=kind;if(kind!=='category')bindPageCategory(props.theme,p,'');
 for(const n of [...props.theme.navigation,...props.theme.categories,...(props.theme.serviceMenuItems||[])])if(n.pageId===p.id)bindPage(props.theme,n,p.id);
 for(const s of props.theme.sections)if(s.pageId===p.id)s.page=pageRoute(p);
 emit('preview',pageRoute(p),p.id);
}
function deletePage(){const p=current.value;if(!p)return;uni.showModal({title:'删除页面',content:'删除“'+p.title+'”及其所有导航入口、栏目和轮播绑定？其他页面会保留。',success:r=>{if(r.confirm){removeContentPage(props.theme,p.id);emit('preview','home','');}}});}
function moveScoped<T>(list:T[],scope:T[],item:T,delta:number){const neighbor=scope[scope.indexOf(item)+delta];if(neighbor!==undefined){const i=list.indexOf(item),j=list.indexOf(neighbor);if(i>=0&&j>=0)[list[i],list[j]]=[list[j],list[i]];}}
function remove<T>(list:T[],item:T){const i=list.indexOf(item);if(i>=0)list.splice(i,1);}
function createModule(p?:ContentPage){
 if(props.theme.sections.length>=40){emit('message','栏目总数最多40个。');return;}
 const s:Section={id:uid('section'),title:'新栏目',subtitle:'',kind:'poster',style:'grid3',source:p?.kind==='category'?'category':'latest',categoryId:p?.categoryId||'',limit:6,columns:3,enabled:true,page:p?pageRoute(p):props.page,orderBy:'latest',showMore:true};
 if(p)s.pageId=p.id;props.theme.sections.push(s);selectedModuleId.value=s.id;tab.value='sections';
}
const sectionFlags=[{key:'showTitle' as const,label:'显示影片标题'},{key:'showScore' as const,label:'显示评分'},{key:'showNote' as const,label:'显示更新备注'},{key:'showYear' as const,label:'显示年份'},{key:'showMore' as const,label:'显示更多入口'}];
function setGeometry(s:Section,key:'gap'|'radius',e:any,min:number,max:number,fallback:number){const value=Number(e.detail.value);s[key]=Number.isFinite(value)?Math.max(min,Math.min(max,Math.round(value))):fallback;}
function checked(e:any){return Boolean(e.detail.value);}
function setCategory(id:string){if(current.value)bindPageCategory(props.theme,current.value,id);}
function setModuleCategory(s:Section,id:string){s.categoryId=id;s.source=id||current.value?.kind==='category'?'category':'latest';}
function setLimit(s:Section,e:any){s.limit=Math.max(1,Math.min(60,Math.trunc(Number(e.detail.value)||6)));}
function categoryLabel(id:string){return props.categories.find(c=>c.id===id)?.name||(id?'分类 #'+id:'未绑定分类');}
const sorts=[{id:'latest',label:'最新更新'},{id:'score',label:'当前页评分'},{id:'hot',label:'内容源热度（需支持）'},{id:'manual',label:'手动片单'}];
</script>
<template><view class="operations-editor">
 <view class="intro"><text class="heading">{{active==='navigation'?'导航与分类绑定':'页面编排'}}</text><text class="muted">当前客户端独立配置。修改后自动保存，已接入客户端下次同步生效。</text></view>
 <OperationsNavigationEditor v-if="active==='navigation'" :theme="theme" :categories="categories" :load-categories="loadCategories" @create="addPage" @message="emit('message',$event)"/>
 <template v-else>
  <view class="page-toolbar"><label class="page-picker">当前页面<EditorChoice :model-value="selectedPage" :options="pageOptions" @update:model-value="preview"/></label><button @click="addPage()">＋ 新建页面</button><button v-if="current" :aria-expanded="settingsOpen" @click="settingsOpen=!settingsOpen">{{settingsOpen?'收起设置':'页面设置'}}</button></view>
  <view v-if="current&&settingsOpen" class="page-settings">
   <view class="fields"><label>页面名称<input v-model="current.title" maxlength="80" placeholder="例如：动漫、暑期活动"/></label><label>页面类型<EditorChoice :model-value="current.kind" :options="pageKinds" @update:model-value="changeKind"/></label></view>
   <label v-if="current.kind==='category'">绑定 CMS 一级分类<CmsCategoryBinding :model-value="current.categoryId" :categories="categories" root-only unbound-label="未绑定 · 暂不显示分类内容" @update:model-value="setCategory"/></label>
   <view class="switch-line"><view><text>启用页面</text><text class="muted">关闭后，其关联入口一并隐藏</text></view><switch :checked="current.enabled" @change="current.enabled=checked($event)"/></view>
   <button class="danger" @click="deletePage">删除此页面及关联入口</button>
  </view>
  <view v-if="!editable" class="empty">这是系统功能页面<text>为它创建独立页面后，可以单独编排内容。</text><button @click="addPage()">＋ 创建独立页面</button></view>
  <view v-else-if="current?.kind==='blank'" class="empty">空白页面<text>保留空白，不自动填充其他页面内容。在页面设置中改为活动或自定义页即可添加栏目。</text></view>
  <template v-else>
   <view class="page-summary"><view><text class="section-title">{{current?.title||'首页'}} · 内容结构</text><text class="muted">{{current?.kind==='category'?categoryLabel(current.categoryId):'独立编排当前页面的轮播与片单'}}</text></view><text class="summary-count">{{boundCarousels.length}} 组轮播 · {{modules.filter(s=>s.enabled).length}} 个已开启栏目</text></view>
   <view class="structure" aria-label="页面内容顺序"><text>导航</text><text>↓</text><text>轮播框架</text><text>↓</text><text>分类快捷入口</text><text>↓</text><text>影片栏目</text></view>
   <text class="muted">结构示意沿用骨架；图片和内容由本页绑定。完整主题联调将在后续接入主题中心时验证。</text>
   <view class="content-tabs" role="tablist"><button role="tab" :aria-selected="tab==='sections'" :class="{chosen:tab==='sections'}" @click="tab='sections'">影片栏目 <text>{{modules.length}}</text></button><button role="tab" :aria-selected="tab==='carousel'" :class="{chosen:tab==='carousel'}" @click="tab='carousel'">已绑定轮播 <text>{{boundCarousels.length}}</text></button></view>
   <template v-if="tab==='sections'">
    <view class="section-header"><view><text class="section-title">影片栏目</text><text class="muted">按列表顺序显示；数量与内容排序分别设置</text></view><button class="primary" @click="createModule(current)">＋ 添加栏目</button></view>
    <view class="workspace"><view class="item-list">
     <button v-for="(s,i) in modules" :key="s.id" class="list-row" :class="{selected:s.id===selectedModuleId}" @click="selectedModuleId=s.id"><text class="number">{{String(i+1).padStart(2,'0')}}</text><view class="row-copy"><text>{{s.title||'未命名栏目'}}</text><text class="muted">{{s.orderBy==='manual'?'手动片单':categoryLabel(s.categoryId)}} · {{s.limit}} 部</text></view><text class="status" :class="{off:!s.enabled}">{{s.enabled?'开启':'关闭'}}</text></button>
     <view v-if="!modules.length" class="empty">尚未添加栏目<text>为当前页面添加分类内容或手动片单。</text></view>
    </view><view class="inspector"><template v-if="selectedModule"><text class="section-title">栏目设置</text><label>栏目标题<input v-model="selectedModule.title" maxlength="80"/></label><label>栏目说明<input v-model="selectedModule.subtitle" maxlength="240" placeholder="选填"/></label><label>内容分类<CmsCategoryBinding :model-value="selectedModule.categoryId" :categories="categories" :unbound-label="current?.kind==='category'?'未绑定 · 暂不显示分类内容':undefined" @update:model-value="setModuleCategory(selectedModule!,$event)"/></label><label>内容排序<EditorChoice v-model="selectedModule.orderBy" :options="sorts"/></label><label v-if="selectedModule.orderBy==='manual'">影片 ID（按输入顺序显示）<input v-model="selectedModule.manualIds" placeholder="例如：123,456,789"/></label><label>显示数量（1–60 部）<input type="number" :value="selectedModule.limit" @blur="setLimit(selectedModule,$event)"/></label><label>栏目排版<EditorChoice v-model="selectedModule.style" :options="[{id:'grid3',label:'宫格'},{id:'grid2',label:'双列宫格（兼容）'},{id:'scroll',label:'单行横滑'},{id:'list',label:'图文列表'},{id:'ranking',label:'排行列表'},{id:'mosaic',label:'拼图'}]"/></label>
<label v-if="!['scroll','list','ranking','mosaic'].includes(selectedModule.style||'grid3')">宫格列数<EditorChoice :model-value="selectedModule.columns" :options="[2,3,4,5,6].map(n=>({id:String(n),label:n+' 列'}))" @update:model-value="selectedModule.columns=Number($event)"/></label>
<label>封面比例<EditorChoice v-model="selectedModule.ratio" :options="[{id:'2/3',label:'竖版 2:3'},{id:'3/4',label:'竖版 3:4'},{id:'16/9',label:'横版 16:9'},{id:'1/1',label:'方形 1:1'}]"/></label>
<label>卡片间距（0–32 px）<input type="number" :value="selectedModule.gap??8" @blur="setGeometry(selectedModule,'gap',$event,0,32,8)"/></label>
<label>封面圆角（0–40 px）<input type="number" :value="selectedModule.radius??theme.layout.posterRadius" @blur="setGeometry(selectedModule,'radius',$event,0,40,theme.layout.posterRadius)"/></label>
<OperationsLayoutFields v-model="selectedModule.appearance" kind="section" title="当前栏目" :text-color="theme.colors.text" :background="theme.colors.background" :accent="theme.colors.accent"/>
<view v-for="flag in sectionFlags" :key="flag.key" class="switch-line"><text>{{flag.label}}</text><switch :checked="flag.key==='showYear'?!!selectedModule[flag.key]:selectedModule[flag.key]!==false" @change="selectedModule[flag.key]=checked($event)"/></view>
<view class="switch-line"><text>显示这个栏目</text><switch :checked="selectedModule.enabled" @change="selectedModule.enabled=checked($event)"/></view><view class="actions"><button :disabled="modules.indexOf(selectedModule)===0" @click="moveScoped(theme.sections,modules,selectedModule,-1)">上移</button><button :disabled="modules.indexOf(selectedModule)===modules.length-1" @click="moveScoped(theme.sections,modules,selectedModule,1)">下移</button><button class="danger" @click="remove(theme.sections,selectedModule)">移除栏目</button></view></template><view v-else class="empty">选择左侧栏目进行编辑</view></view></view>
   </template>
   <template v-else>
    <view class="section-header"><view><text class="section-title">已绑定轮播模块</text><text class="muted">引用轮播图中心的绑定；请前往轮播图中心编辑图片、跳转及开关。</text></view></view>
    <view class="item-list"><view v-for="g in boundCarousels" :key="g.id" class="list-row"><view class="row-copy"><text>{{g.name||'未命名轮播组'}}</text><text class="muted">{{g.entryIds.map(id=>theme.categories.find(e=>e.id===id)?.label||id).join('、')}} · {{g.slides.filter(s=>s.enabled).length}} 张已开启轮播</text></view><text class="status" :class="{off:!g.enabled}">{{g.enabled?'开启':'关闭'}}</text></view><view v-if="!boundCarousels.length" class="empty">暂未绑定轮播模块<text>在轮播图中心绑定本页对应的 APP 一级分类。</text></view></view>
   </template>
  </template>
 </template>
</view></template>
<style scoped>
.operations-editor{display:flex;flex-direction:column;gap:20px;color:var(--studio-text,#d8e3df)}.intro{padding-bottom:16px;border-bottom:1px solid var(--studio-border,#ffffff12)}.heading{display:block;font-size:18px;font-weight:600;letter-spacing:-.3px}.muted{display:block;font-size:11px;line-height:1.8;color:var(--studio-muted,#91a9a0);margin-top:4px}.page-toolbar,.page-summary,.section-header,.switch-line{display:flex;align-items:center;justify-content:space-between;gap:12px}.page-picker{flex:1;min-width:180px}.page-toolbar{align-items:flex-end;flex-wrap:wrap}.page-settings{display:flex;flex-direction:column;gap:16px;padding:20px;border:1px solid var(--studio-border,#ffffff12);border-radius:10px;background:var(--studio-panel,#111b21)}.fields{display:grid;grid-template-columns:1fr 1fr;gap:16px}.section-title{font-size:13px;font-weight:600}.summary-count{font-size:10px;color:var(--studio-muted,#91a9a0);white-space:nowrap}.structure{display:flex;align-items:center;flex-wrap:wrap;gap:12px;padding:14px 16px;background:var(--studio-input,#172329);border-radius:8px;font-size:11px}.structure>text:nth-child(even){color:var(--studio-muted,#91a9a0);transform:rotate(-90deg)}.content-tabs{display:flex;gap:20px;border-bottom:1px solid var(--studio-border,#ffffff12)}.content-tabs button{border:0;border-radius:0;background:transparent;padding:8px 0;color:var(--studio-muted,#91a9a0);border-bottom:2px solid transparent}.content-tabs button.chosen{border-color:var(--studio-link,#c9eaa2);color:var(--studio-text,#d8e3df)}.content-tabs button text{font-size:10px;margin-left:5px;opacity:.6}.workspace{display:grid;grid-template-columns:minmax(210px,1fr) minmax(250px,1fr);border:1px solid var(--studio-border,#ffffff12);border-radius:12px;overflow:hidden}.item-list{padding:8px;background:var(--studio-panel,#111b21);min-width:0}.list-row{width:100%;display:flex;align-items:center;gap:12px;padding:12px 10px;margin:2px 0;border-color:transparent;background:transparent;text-align:left;min-height:64px;line-height:1.5}.list-row.selected{border-color:var(--studio-link,#c9eaa2);background:#91b3760c}.row-copy{flex:1;min-width:0}.row-copy>text{display:block;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.number{font-size:10px;font-variant-numeric:tabular-nums;color:var(--studio-muted,#91a9a0)}.status{font-size:10px;color:var(--studio-link,#c9eaa2)}.status.off{color:var(--studio-muted,#91a9a0)}.inspector{padding:20px;border-left:1px solid var(--studio-border,#ffffff12);display:flex;flex-direction:column;gap:16px;min-width:0}.actions{display:flex;gap:6px;flex-wrap:wrap}.empty{padding:36px 16px;font-size:12px;line-height:1.8;text-align:center;color:var(--studio-muted,#91a9a0)}.empty>text{display:block;font-size:11px;margin:8px 0}.empty button{margin:16px auto 0}.frame-switch{padding:0 4px;font-size:12px}label{display:flex;flex-direction:column;gap:8px;font-size:11px;min-width:0}input{height:40px;box-sizing:border-box;padding:0 11px;border:1px solid var(--studio-border,#ffffff12);border-radius:6px;background:var(--studio-input,#172329);color:inherit;font-size:12px}button{margin:0;padding:4px 12px;border:1px solid var(--studio-border,#ffffff12);border-radius:6px;background:var(--studio-input,#172329);color:inherit;font-size:11px;line-height:28px;cursor:pointer}button::after{border:0}button.primary{background:var(--studio-action,#26372d);color:var(--studio-link,#c9eaa2)}button[disabled]{opacity:.35;cursor:default}.danger{color:#e8a99d}button:focus-visible{outline:2px solid var(--studio-link,#c9eaa2);outline-offset:3px}switch{transform:scale(.8);transform-origin:right center}@media(hover:hover) and (pointer:fine){button:hover{border-color:var(--studio-link,#c9eaa2)}}@media(max-width:1080px){.workspace{grid-template-columns:1fr}.inspector{border-left:0;border-top:1px solid var(--studio-border,#ffffff12)}.item-list{max-height:320px;overflow:auto}}@media(max-width:600px){.fields{grid-template-columns:1fr}.page-summary{align-items:flex-start;flex-direction:column}.structure{gap:8px}}
</style>
