<script setup lang="ts">
import {computed,ref,watch} from 'vue';
import {setBottomEnabled} from '@/domain/navigation-modules';
import type {AppPage,Category,NavItem,Theme,NavigationDisplay,MenuTargetType} from '@/domain/types';
import {pageNames} from '@/domain/presentation';
import {ensurePrimaryContentPage,renamePrimaryEntry,bindPrimaryCategory,detachPrimaryPage,reconcileCarouselReferences,bindPage,bindPageCategory,findContentPage,serviceMenuEntries,visibleCategoryShortcuts,migrateCategoryShortcutOwners,menuTargetType,setMenuTargetType,menuExternalUrl} from '@/domain/operation-pages';
import CmsCategoryBinding from './CmsCategoryBinding.vue';
import NavigationArtwork from './NavigationArtwork.vue';
import EntryTextColor from './EntryTextColor.vue';
import EditorChoice from './EditorChoice.vue';
import OperationsLayoutFields from './OperationsLayoutFields.vue';
import OperationsShortcutsEditor from './OperationsShortcutsEditor.vue';
const props=defineProps<{theme:Theme;categories:Category[];loadCategories:()=>Promise<Category[]>}>();
const emit=defineEmits<{create:[entry?:NavItem];message:[message:string]}>();
type Group='categories'|'navigation'|'serviceMenuItems'|'shortcuts';
const group=ref<Group>('categories'),selectedId=ref(''),editing=ref(false);
const groups=[{id:'categories' as Group,label:'一级分类',hint:'APP 顶部的分类与活动入口'}, {id:'navigation' as Group,label:'底部导航',hint:'固定功能页面：排期与排行覆盖全站，短剧为上下滑动播放器'}, {id:'serviceMenuItems' as Group,label:'左上角菜单',hint:'三条杠展开后的自定义入口'}, {id:'shortcuts' as Group,label:'中间导航',hint:'轮播与影片之间的分类入口'}];
const entries=computed(()=>group.value==='shortcuts'?[]:group.value==='serviceMenuItems'?serviceMenuEntries(props.theme):props.theme[group.value]);
const selected=computed(()=>entries.value.find(n=>n.id===selectedId.value));
const description=computed(()=>groups.find(g=>g.id===group.value)!.hint);
const targets=computed(()=>[...(props.theme.contentPages||[]).map(p=>({id:'content:'+p.id,label:p.title+' · '+({category:'分类页',campaign:'活动页',blank:'空白页',custom:'自定义页'}[p.kind])})),...Object.entries(pageNames).map(([id,label])=>({id:'route:'+id,label:'系统 · '+label}))]);
const menuTargetOptions=[{id:'system',label:'系统功能'},{id:'page',label:'内置自定义页面'},{id:'external',label:'外部网页'}];
const systemTargets=Object.entries(pageNames).map(([id,label])=>({id:'route:'+id,label}));
const pageTargets=computed(()=>targets.value.filter(t=>t.id.startsWith('content:')));
const style=computed(()=>props.theme.navigationStyle||{});
watch(()=>entries.value.map(n=>n.id).join(','),()=>{if(!selected.value){selectedId.value='';editing.value=false;}});
function selectGroup(id:Group){group.value=id;selectedId.value='';editing.value=false;}
function writable(){if(group.value==='serviceMenuItems'&&props.theme.serviceMenuItems===undefined)props.theme.serviceMenuItems=serviceMenuEntries(props.theme);return entries.value;}
function selectEntry(n:NavItem,edit=false){if(edit)writable();selectedId.value=n.id;editing.value=edit;}
function toggleEntry(n:NavItem,enabled:boolean){const entry=writable().find(x=>x.id===n.id);if(entry){if(group.value==='navigation')setBottomEnabled(props.theme,entry,enabled);else entry.enabled=enabled;}}
function setTextColor(n:NavItem,value:string|undefined){const entry=writable().find(x=>x.id===n.id);if(!entry)return;if(value===undefined)delete entry.textColor;else if(/^#[0-9a-fA-F]{6}$/.test(value))entry.textColor=value;}
function canBind(n:NavItem){return group.value!=='navigation'&&!(group.value==='serviceMenuItems'&&menuTargetType(n)==='external')&&(group.value==='categories'||(n.pageId?findContentPage(props.theme,n.pageId)?.kind==='category':['category','ranking','schedule','shortdrama'].includes(n.page)));}
function boundId(n:NavItem){return group.value==='categories'?n.categoryId||'':n.pageId?findContentPage(props.theme,n.pageId)?.categoryId||'':n.categoryId||'';}
function bindingLabel(n:NavItem){const id=boundId(n);return id?'已绑定：'+(props.categories.find(c=>c.id===id)?.name||'分类 #'+id):'未绑定';}
function bindCategory(n:NavItem,id:string){const entry=writable().find(x=>x.id===n.id);if(!entry)return;const page=findContentPage(props.theme,entry.pageId);if(page&&group.value!=='categories')bindPageCategory(props.theme,page,id);else if(group.value==='categories')bindPrimaryCategory(props.theme,entry,id);else entry.categoryId=id;selectedId.value=entry.id;}
const modes=[{id:'text',label:'纯文字'},{id:'icon',label:'纯图标'},{id:'both',label:'图标＋文字'}];
const slots=computed(()=>[{id:'',label:'主题对应图标'},...[...props.theme.navigation,...props.theme.categories,...serviceMenuEntries(props.theme)].filter((n,i,a)=>a.findIndex(x=>x.id===n.id)===i).map(n=>({id:n.id,label:n.label+' · '+n.id}))]);
function count(id:Group){if(id==='shortcuts')return props.theme.categories.filter(n=>visibleCategoryShortcuts(props.theme,n.id).length>0).length;return (id==='serviceMenuItems'?serviceMenuEntries(props.theme):props.theme[id]).filter(n=>n.enabled).length;}
function display(n:NavItem){return group.value==='categories'?'text':n.display||(props.theme.pageStyles.navigation==='text'?'text':'both');}
function target(n:NavItem){return n.pageId?'content:'+n.pageId:'route:'+n.page;}
function targetLabel(n:NavItem){if(group.value==='serviceMenuItems'&&menuTargetType(n)==='external')return n.url||'待填写网页地址';if(group.value==='navigation')return ({home:'固定首页',shortdrama:'上下滑动播放短剧',member:'固定会员页',schedule:'全站总排期表',ranking:'全站总排行榜',mine:'固定个人中心'} as Partial<Record<AppPage,string>>)[n.page]||pageNames[n.page];const page=findContentPage(props.theme,n.pageId);const category=props.categories.find(c=>c.id===n.categoryId);return page?page.title:((pageNames[n.page]||'未绑定页面')+(category?' · '+category.name:''));}
function setTarget(value:string|number){const n=selected.value;if(!n)return;const id=String(value);if(group.value==='serviceMenuItems'){delete n.url;n.targetType=id.startsWith('content:')?'page':'system';}if(id.startsWith('content:'))bindPage(props.theme,n,id.slice(8));else{detachPrimaryPage(props.theme,n);delete n.pageId;n.page=id.slice(6) as AppPage;if(group.value!=='categories')n.categoryId='';}}
function add(){if(entries.value.length>=20){emit('message','每组导航最多20项。');return;}const n:NavItem={id:'nav-'+Date.now().toString(36)+'-'+Math.random().toString(36).slice(2,7),label:'新入口',page:group.value==='categories'?'category':'home',categoryId:'',enabled:false,icon:'',activeIcon:'',...(group.value==='categories'?{display:'text' as const}:{})};if(group.value==='categories'&&!ensurePrimaryContentPage(props.theme,n)){emit('message','独立页面最多40个，请先清理不用的页面再添加一级分类。');return;}writable().push(n);selectedId.value=n.id;editing.value=true;}
function move(n:NavItem,delta:number){const list=writable(),i=list.findIndex(x=>x.id===n.id),j=i+delta;if(i>=0&&j>=0&&j<list.length)[list[i],list[j]]=[list[j],list[i]];}
function remove(n:NavItem){uni.showModal({title:'移除入口',content:'移除“'+n.label+'”入口？已绑定页面和内容会保留。',success:r=>{if(r.confirm){const list=writable(),i=list.findIndex(x=>x.id===n.id);if(i>=0){if(group.value==='categories'){migrateCategoryShortcutOwners(props.theme);detachPrimaryPage(props.theme,n);}list.splice(i,1);if(group.value==='categories'){props.theme.categoryShortcuts=props.theme.categoryShortcuts?.filter(g=>g.entryId!==n.id);reconcileCarouselReferences(props.theme);}}}}});}
function renameEntry(value:string){const n=selected.value;if(!n)return;if(group.value==='categories')renamePrimaryEntry(props.theme,n,value);else n.label=value;}
function renameInput(event:Event){renameEntry((event as CustomEvent<{value:string}>).detail.value);}
function checked(e:any){return Boolean(e.detail.value);}
</script>
<template><view class="nav-workspace">
 <view class="group-tabs" role="tablist" aria-label="选择导航位置"><button v-for="g in groups" :key="g.id" role="tab" :aria-selected="group===g.id" :class="{chosen:group===g.id}" @click="selectGroup(g.id)">{{g.label}}<text>{{count(g.id)}}</text></button></view>
 <OperationsShortcutsEditor v-if="group==='shortcuts'" :theme="theme" :categories="categories"/>
 <template v-else>
 <OperationsLayoutFields v-if="group==='serviceMenuItems'" v-model="theme.serviceMenuStyle" kind="menu" title="左上角菜单排版" :text-color="theme.colors.text" :background="theme.colors.background" :accent="theme.colors.accent"/>
 <view class="group-header"><view><text class="section-title">{{groups.find(g=>g.id===group)?.label}}</text><text class="muted">{{description}}</text></view><button v-if="group!=='navigation'" class="primary" @click="add">＋ 添加入口</button></view>
 <view v-if="group==='serviceMenuItems'" class="menu-switch"><view><text>显示左上角菜单</text><text class="muted">关闭后隐藏整个入口，保留菜单配置</text></view><switch :checked="theme.features.serviceMenu" @change="theme.features.serviceMenu=checked($event)"/></view>
 <view class="navigation-preview" aria-label="导航实时预览"><button v-for="n in entries.filter(n=>n.enabled)" :key="n.id" :class="{chosen:n.id===selectedId}" :style="{fontSize:style.fontSize?style.fontSize+'px':undefined,color:n.id===selectedId?style.activeColor:style.color}"  :aria-label="n.label" @click="selectEntry(n)"><NavigationArtwork v-if="display(n)!=='text'" :theme="theme" :entry="n" :active="n.id===selectedId" :size="20" :color="n.id===selectedId?style.activeColor:style.color"/><text v-if="display(n)!=='icon'" :style="{color:n.textColor}">{{n.label}}</text></button><text v-if="!entries.some(n=>n.enabled)" class="muted">暂无已开启入口</text></view>
 <view class="entry-workspace">
  <view class="entry-list" aria-label="导航入口列表"><view class="list-caption"><text>入口 / 绑定分类</text><text>显示 · 排序 · 操作</text></view>
   <view v-for="(n,i) in entries" :key="n.id" class="entry-row" :class="{selected:n.id===selectedId}"><text class="number">{{String(i+1).padStart(2,'0')}}</text><button class="entry-copy" :aria-label="'预览'+n.label" @click="selectEntry(n)"><text class="entry-name" :style="{color:n.textColor}">{{n.label||'未命名入口'}}</text><text class="target-name">{{canBind(n)?bindingLabel(n):targetLabel(n)}}</text></button><view class="row-actions"><EntryTextColor :label="n.label||'未命名入口'" :model-value="n.textColor" :default-color="style.color||theme.colors.text" @update:model-value="setTextColor(n,$event)"/><switch :aria-label="n.label+'显示开关'" :checked="n.enabled" @change="toggleEntry(n,checked($event))"/><button :aria-label="'上移'+n.label" :disabled="i===0" @click="move(n,-1)">↑</button><button :aria-label="'下移'+n.label" :disabled="i===entries.length-1" @click="move(n,1)">↓</button><CmsCategoryBinding v-if="canBind(n)" :model-value="boundId(n)" :categories="categories" :load-categories="loadCategories" root-only compact unbound-label="解除绑定 · 暂不显示分类内容" @update:model-value="bindCategory(n,$event)"/><button @click="selectEntry(n,true)">编辑</button><button v-if="group!=='navigation'" class="danger" :aria-label="'移除'+n.label" @click="remove(n)">移除</button></view></view>
   <view v-if="!entries.length" class="empty">暂无入口<text>添加后可绑定分类、活动或自定义页面</text></view>
  </view>
  <view v-if="editing&&selected" class="entry-inspector"><template v-if="selected"><view class="inspector-head"><text class="section-title">编辑入口</text><button @click="editing=false">收起编辑</button></view>
   <template v-if="group!=='categories'"><label>显示方式<EditorChoice :model-value="selected.display||(theme.pageStyles.navigation==='text'?'text':'both')" :options="modes" @update:model-value="selected.display=String($event) as NavigationDisplay"/></label><label>引用主题图标位<EditorChoice :model-value="selected.iconKey||''" :options="slots" @update:model-value="selected.iconKey=String($event)"/></label><text class="muted">图片、SVG 和阿里矢量图在主题中心替换，这里选择显示方式及图标位。</text></template><label>APP 显示名称<input :value="selected.label" @input="renameInput" maxlength="80" placeholder="例如：动漫、暑期活动"/></label>
   <template v-if="group==='serviceMenuItems'">
    <label>打开目标<EditorChoice :model-value="menuTargetType(selected)" :options="menuTargetOptions" @update:model-value="setMenuTargetType(theme,selected!,String($event) as MenuTargetType)"/></label>
    <label v-if="menuTargetType(selected)==='system'">系统功能<EditorChoice :model-value="target(selected)" :options="systemTargets" @update:model-value="setTarget"/></label>
    <template v-else-if="menuTargetType(selected)==='page'">
     <label>内置自定义页面<EditorChoice :model-value="selected.pageId?'content:'+selected.pageId:''" :options="pageTargets" @update:model-value="setTarget"/></label>
     <text class="muted">{{pageTargets.length?'选择已在页面编排中创建的页面，页面关闭时此入口同步隐藏。':'暂无可选页面，请先在页面编排中创建。'}}</text>
    </template>
    <template v-else>
     <label>网页地址<input v-model.trim="selected.url" type="text" maxlength="2048" placeholder="https://example.com/activity" aria-label="外部网页地址"/></label>
     <text class="muted">{{menuExternalUrl(selected.url)?'点击入口后打开该网页。':'请填写完整的 http:// 或 https:// 网页地址。'}}</text>
    </template>
   </template>
   <label v-else-if="group==='categories'">页面绑定<EditorChoice :model-value="target(selected)" :options="targets" @update:model-value="setTarget"/></label>
   <label v-if="canBind(selected)">绑定 CMS 一级分类<CmsCategoryBinding  :model-value="boundId(selected)" @update:model-value="bindCategory(selected,$event)" :categories="categories" :load-categories="loadCategories" root-only unbound-label="未绑定 · 暂不显示分类内容"/></label>
   <text v-if="group!=='serviceMenuItems'" class="muted">{{group==='categories'?'CMS 绑定决定影视来源，页面绑定决定打开的页面，两项分别保存。':'入口名称独立于 CMS 名称。分类按真实 ID 读取内容。'}}</text>


   <text v-if="group==='navigation'" class="muted">{{targetLabel(selected)}} · 功能与页面固定，显示名称、颜色、图标和排序可调整。</text>
  </template><view v-else class="empty">选择左侧入口<text>在这里修改名称、绑定与显示状态</text></view></view>
 </view>
</template>
</view></template>
<style scoped>
.nav-workspace{display:flex;flex-direction:column;gap:20px}.group-tabs{display:flex;gap:4px;padding:4px;border:1px solid var(--studio-border,#ffffff12);border-radius:10px;background:var(--studio-input,#172329)}.group-tabs button{flex:1;border:0;background:transparent;color:var(--studio-muted,#91a9a0);white-space:nowrap}.group-tabs button.chosen{background:var(--studio-panel,#111b21);color:var(--studio-text,#d8e3df);box-shadow:0 2px 6px #0002}.group-tabs button>text{margin-left:8px;font-size:10px;opacity:.65}.group-header,.menu-switch,.inspector-head,.list-caption{display:flex;justify-content:space-between;align-items:center;gap:12px}.section-title{font-weight:600;font-size:13px;line-height:1.6}.muted{display:block;font-size:11px;line-height:1.7;color:var(--studio-muted,#91a9a0);margin-top:4px}.menu-switch{font-size:12px}.entry-workspace{display:grid;grid-template-columns:1fr;border:1px solid var(--studio-border,#ffffff12);border-radius:12px;overflow:hidden}.entry-list{min-width:0;background:var(--studio-panel,#111b21);padding:8px}.list-caption{padding:8px 10px 12px;font-size:10px;color:var(--studio-muted,#91a9a0)}.entry-row{display:flex;align-items:center;gap:10px;width:100%;padding:12px 10px;margin:2px 0;text-align:left;border:1px solid transparent;background:transparent;line-height:1.4;min-height:64px}.entry-row.selected{border-color:var(--studio-link,#c9eaa2);background:#91b3760c}.number{font-size:10px;color:var(--studio-muted,#91a9a0);font-variant-numeric:tabular-nums}.entry-copy{flex:1;min-width:0}.entry-name,.target-name{display:block;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.entry-name{font-size:12px}.target-name{font-size:10px;margin-top:5px;color:var(--studio-muted,#91a9a0)}.entry-inspector{display:flex;flex-direction:column;gap:16px;padding:20px;border-top:1px solid var(--studio-border,#ffffff12);min-width:0}.inspector-head{padding-bottom:12px;border-bottom:1px solid var(--studio-border,#ffffff12)}.empty{padding:40px 12px;text-align:center;font-size:12px;color:var(--studio-muted,#91a9a0)}.empty text{display:block;font-size:11px;line-height:1.8;margin-top:8px}label{display:flex;flex-direction:column;gap:8px;font-size:11px;min-width:0}input{height:40px;padding:0 11px;border:1px solid var(--studio-border,#ffffff12);border-radius:6px;background:var(--studio-input,#172329);font-size:12px;color:inherit;box-sizing:border-box}button{margin:0;padding:4px 12px;border:1px solid var(--studio-border,#ffffff12);border-radius:6px;background:var(--studio-input,#172329);color:inherit;font-size:11px;line-height:28px;cursor:pointer}button::after{border:0}@media(hover:hover) and (pointer:fine){button:hover{border-color:var(--studio-link,#c9eaa2)}}button:focus-visible{outline:2px solid var(--studio-link,#c9eaa2);outline-offset:3px}button[disabled]{opacity:.35;cursor:default}button.primary{background:var(--studio-action,#26372d);color:var(--studio-link,#c9eaa2)}.danger{color:#e8a99d}switch{transform:scale(.8);transform-origin:right center}@media(max-width:1080px){.entry-workspace{grid-template-columns:1fr}.entry-inspector{border-left:0;border-top:1px solid var(--studio-border,#ffffff12)}.entry-list{max-height:350px;overflow:auto}}@media(max-width:600px){.group-tabs button{padding:5px 6px;font-size:10px}.group-tabs button>text{display:none}}
.entry-row{display:flex;align-items:center;gap:10px;padding:10px;min-height:58px;border:1px solid transparent;border-radius:8px}.entry-copy{border:0;background:transparent;text-align:left;padding:0;line-height:1.5}.row-actions{display:flex;align-items:center;gap:6px;flex-shrink:0}.row-actions button{min-height:38px;padding:4px 10px}.navigation-preview{display:flex;flex-wrap:wrap;gap:8px;padding:12px;border:1px solid var(--studio-border,#ffffff12);border-radius:10px}.navigation-preview button{display:flex;align-items:center;justify-content:center;gap:6px}.navigation-preview .chosen{color:var(--studio-link,#c9eaa2);border-color:var(--studio-link,#c9eaa2)}@media(max-width:680px){.entry-row{flex-wrap:wrap}.row-actions{width:100%;justify-content:flex-end}.entry-list{max-height:none}.row-actions button{padding:4px 8px}}
</style>
