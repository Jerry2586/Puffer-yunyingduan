<script setup lang="ts">
import {computed,onUnmounted,ref,watch} from 'vue';
import type {AppPage,Category,CarouselTarget,OperationsCarouselGroup,OperationsCarouselSlide,Theme,Vod} from '@/domain/types';
import {api,assetUrl,chooseLocalFile} from '@/services/api';
import {pageNames} from '@/domain/presentation';
import {menuExternalUrl} from '@/domain/operation-pages';
import {carouselGroupIssue,carouselImageValid,carouselSlideIssue,carouselTargetOptions,resetCarouselTarget,resolveCarouselRecord,carouselFilmImage,syncCarouselGroupName} from '@/domain/operation-carousels';
import EditorChoice from './EditorChoice.vue';
const props=defineProps<{theme:Theme;categories:Category[];loadCategories:()=>Promise<Category[]>}>();
const emit=defineEmits<{message:[value:string]}>();
const groups=computed(()=>props.theme.carouselGroups||[]);
const expandedId=ref(''),bindingId=ref(''),editingNameId=ref('');
const bindingGroup=computed(()=>groups.value.find(g=>g.id===bindingId.value));
const nameDrafts=ref<Record<string,string>>({});
const switchRevision=ref(0);
watch(()=>groups.value.map(g=>g.id),ids=>{
 if(!ids.includes(expandedId.value))expandedId.value='';
 if(!ids.includes(bindingId.value))bindingId.value='';
 if(!ids.includes(editingNameId.value))editingNameId.value='';
 for(const id of Object.keys(nameDrafts.value))if(!ids.includes(id))delete nameDrafts.value[id];
});
watch(()=>[groups.value.map(g=>[g.id,g.name,g.nameMode,...g.entryIds]),props.theme.categories.map(e=>[e.id,e.label])],()=>{
 for(const group of groups.value)syncCarouselGroupName(props.theme,group);
},{immediate:true});
function groupName(group:OperationsCarouselGroup){return nameDrafts.value[group.id]??group.name;}
function toggleExpanded(groupId:string){expandedId.value=expandedId.value===groupId?'':groupId;}
function bindingLabel(group:OperationsCarouselGroup){const entry=props.theme.categories.find(e=>group.entryIds.includes(e.id));return entry?'已绑定：'+entry.label+(!entry.enabled?'（分类已关闭）':''):'未绑定一级分类';}
function checked(event:any){return !!event.detail.value;}

const pageOptions=computed(()=>[{id:'',label:'选择已创建的内容页'},...(props.theme.contentPages||[]).map(p=>({id:p.id,label:p.title+(p.enabled?'':'（已关闭）')}))]);
const systemOptions=Object.entries(pageNames).map(([id,label])=>({id,label}));
function uid(prefix:string){return prefix+'-'+Date.now().toString(36)+'-'+Math.random().toString(36).slice(2,8);}
function current(groupId:string,slideId:string){return live?resolveCarouselRecord(props.theme,groupId,slideId):undefined;}
function addGroup(){
 if(groups.value.length>=40){emit('message','轮播组最多40组。');return;}
 const group:OperationsCarouselGroup={id:uid('carousel'),name:'新轮播组',nameMode:'auto',enabled:false,entryIds:[],slides:[]};
 if(!props.theme.carouselGroups)props.theme.carouselGroups=[];
 props.theme.carouselGroups.push(group);expandedId.value=group.id;
}
function removeGroup(group:OperationsCarouselGroup){uni.showModal({title:'删除轮播组',content:'删除“'+group.name+'”及其轮播内容，并解除一级分类绑定？',success:r=>{if(r.confirm&&live)props.theme.carouselGroups=groups.value.filter(g=>g.id!==group.id);}});}
function openName(groupId:string){
 if(editingNameId.value&&editingNameId.value!==groupId)finishName(editingNameId.value);
 if(groups.value.some(g=>g.id===groupId))editingNameId.value=groupId;
}
function finishName(groupId:string){
 const group=groups.value.find(g=>g.id===groupId);
 if(group)rename(groupId,groupName(group),true);
 if(editingNameId.value===groupId)editingNameId.value='';
}
function rename(groupId:string,value:string,finish=false){
 const group=live?groups.value.find(g=>g.id===groupId):undefined;if(!group)return;
 nameDrafts.value[groupId]=value;
 const name=value.trim(),valid=!!name&&new TextEncoder().encode(name).length<=240;
 if(valid&&name!==group.name){group.nameMode='custom';group.name=name;}
 if(finish){if(!valid)emit('message','轮播组名称不能为空，最多80个汉字。');delete nameDrafts.value[groupId];if(editingNameId.value===groupId)editingNameId.value='';}
}
function owner(entryId:string){return groups.value.find(g=>g.entryIds.includes(entryId));}
function bind(group:OperationsCarouselGroup,entryId:string){
 const target=live?groups.value.find(g=>g.id===group.id):undefined;if(!target)return;
 if(!props.theme.categories.some(e=>e.id===entryId)){emit('message','该一级分类已不存在，请重新选择。');return;}
 const existing=owner(entryId);if(existing&&existing.id!==target.id){emit('message','该分类已绑定“'+existing.name+'”，请先解除原绑定。');return;}
 syncCarouselGroupName(props.theme,target);
 target.entryIds=target.entryIds.includes(entryId)?[]:[entryId];
 syncCarouselGroupName(props.theme,target);
 if(!target.entryIds.length)target.enabled=false;
}
function toggleGroup(group:OperationsCarouselGroup,on:boolean){switchRevision.value++;const target=live?groups.value.find(g=>g.id===group.id):undefined;if(!target)return;const issue=on?carouselGroupIssue(props.theme,target):'';if(issue){emit('message',issue);return;}target.enabled=on;}
function addSlide(group:OperationsCarouselGroup){
 if(group.slides.length>=20||groups.value.reduce((sum,g)=>sum+g.slides.length,0)>=200){emit('message','每组最多20条，所有轮播组合计最多200条。');return;}
 group.slides.push({id:uid('slide'),title:'',image:'',sourceVodId:'',enabled:false,target:{type:'vod',vodId:''}});
}
function removeSlide(group:OperationsCarouselGroup,slide:OperationsCarouselSlide){uni.showModal({title:'删除轮播内容',content:'删除这条轮播内容？',success:r=>{const record=current(group.id,slide.id);if(r.confirm&&record){record.group.slides=record.group.slides.filter(s=>s.id!==slide.id);}}});}
function move(group:OperationsCarouselGroup,index:number,offset:number){const next=index+offset;if(next<0||next>=group.slides.length)return;const copy=[...group.slides];[copy[index],copy[next]]=[copy[next],copy[index]];group.slides=copy;}
function toggleSlide(group:OperationsCarouselGroup,slide:OperationsCarouselSlide,on:boolean){switchRevision.value++;const record=current(group.id,slide.id);if(!record)return;const issue=on?carouselSlideIssue(props.theme,record.slide):'';if(issue){emit('message',issue);return;}record.slide.enabled=on;}
function changeTarget(group:OperationsCarouselGroup,slide:OperationsCarouselSlide,type:CarouselTarget['type']){if(slide.target.type===type)return;resetCarouselTarget(slide,type);}
function setPage(group:OperationsCarouselGroup,slide:OperationsCarouselSlide,pageId:string){slide.target={type:'page',pageId};if(!pageId){slide.enabled=false;}}
function setImage(group:OperationsCarouselGroup,slide:OperationsCarouselSlide,value:string){const image=value.trim();if(image&&!carouselImageValid(image)){emit('message','图片须使用完整 HTTP(S) 地址或已上传图片。');return;}slide.image=image;slide.sourceVodId='';if(!image){slide.enabled=false;}}
function setExternal(group:OperationsCarouselGroup,slide:OperationsCarouselSlide,value:string){const url=value.trim();if(url&&!menuExternalUrl(url)){emit('message','请填写完整 HTTP(S) 链接，链接不能包含账号密码或空格。');return;}slide.target={type:'external',url};if(!url){slide.enabled=false;}}
function setTitle(slide:OperationsCarouselSlide,value:string){if(new TextEncoder().encode(value).length>240){emit('message','标题最多80个汉字。');return;}slide.title=value;}
const uploading=ref('');let live=true;
onUnmounted(()=>{live=false;request++;});
async function upload(group:OperationsCarouselGroup,slide:OperationsCarouselSlide){
 if(uploading.value)return;uploading.value=slide.id;
 try{const file=await chooseLocalFile('image/png,image/jpeg,image/webp');if(!current(group.id,slide.id))return;const result=await api.asset(file.name,file.data);const record=current(group.id,slide.id);if(record){record.slide.image=result.url;record.slide.sourceVodId='';emit('message','图片已上传，将自动保存。');}}
 catch(error){if(live)emit('message',(error as Error).message);}
 finally{if(live)uploading.value='';}
}
const picker=ref<{groupId:string;slideId:string;mode:'display'|'target'}>();
const keyword=ref(''),categoryId=ref(''),films=ref<Vod[]>([]),loading=ref(false),error=ref(''),filmPage=ref(1),filmPages=ref(1),total=ref(0);
const categoryOptions=computed(()=>[{id:'',label:'全部 CMS 分类'},...props.categories.map(c=>({id:c.id,label:c.name}))]);
let request=0;
async function openFilms(group:OperationsCarouselGroup,slide:OperationsCarouselSlide,mode:'display'|'target'){
 picker.value={groupId:group.id,slideId:slide.id,mode};keyword.value='';categoryId.value='';films.value=[];filmPage.value=1;filmPages.value=1;total.value=0;
 void searchFilms(1);
 if(!props.categories.length){try{await props.loadCategories();}catch{/* The catalog request shows connection errors; filtering is optional. */}}
}
function closeFilms(){picker.value=undefined;request++;loading.value=false;}
async function searchFilms(page=1){
 if(!picker.value)return;const token=++request;loading.value=true;error.value='';films.value=[];
 try{const result=await api.catalog(page,keyword.value.trim(),categoryId.value);if(!live||token!==request||!picker.value)return;films.value=result.items;filmPage.value=result.page;filmPages.value=Math.max(1,result.pages);total.value=result.total;}
 catch(e){if(live&&token===request)error.value=(e as Error).message;}
 finally{if(live&&token===request)loading.value=false;}
}
function chooseFilm(vod:Vod){
 const dest=picker.value;const record=dest&&current(dest.groupId,dest.slideId);if(!dest||!record){closeFilms();return;}
 const slide=record.slide;
 if(!/^\d{1,20}$/.test(vod.id)){emit('message','CMS 返回的影片编号无效。');return;}
 if(dest.mode==='display'){
  const image=carouselFilmImage(vod);
  if(!carouselImageValid(image)){emit('message','该影片没有可用图片，请选择其他影片或自行上传。');return;}
  if(new TextEncoder().encode(vod.title).length>240){emit('message','CMS 影片标题过长，请使用自定义图片和标题。');return;}
  slide.sourceVodId=vod.id;slide.image=image;slide.title=vod.title;
  if(slide.target.type==='vod'&&!slide.target.vodId)slide.target={type:'vod',vodId:vod.id};
 }else slide.target={type:'vod',vodId:vod.id};
 closeFilms();
}
</script>
<template>
<view class="carousel-center">
 <view class="heading"><view><text class="title">轮播图中心</text><text class="hint">名称与配置自动保存。绑定一级分类后，展开模块添加轮播内容。</text></view><button class="primary" @click="addGroup">＋ 新建轮播组</button></view>
 <view v-if="!groups.length" class="empty"><text class="empty-title">还没有轮播组</text><text class="hint">先创建一组轮播，再选择要展示它的一级分类。</text><button @click="addGroup">新建轮播组</button></view>
 <view v-else class="group-list">
  <view v-for="(group,groupIndex) in groups" :key="group.id" class="group-module" :class="{expanded:expandedId===group.id}">
   <view class="group-row">
    <text class="number">{{String(groupIndex+1).padStart(2,'0')}}</text>
    <view class="group-copy">
     <view class="group-name-row">
      <input v-if="editingNameId===group.id" class="rename-input" :aria-label="'轮播组 '+(groupIndex+1)+'名称'" :value="groupName(group)" :focus="true" maxlength="80" placeholder="输入轮播组名称" confirm-type="done" @input="rename(group.id,($event as any).detail.value)" @blur="rename(group.id,($event as any).detail.value,true)" @confirm="finishName(group.id)"/>
      <template v-else>
       <text class="group-name-label">{{group.name}}</text>
       <button class="rename-button" :aria-label="'修改轮播组名称：'+group.name" title="修改名称" @click="openName(group.id)">🖊</button>
      </template>
     </view>
     <text class="group-meta">{{bindingLabel(group)}} · {{group.slides.length}} 条轮播</text>
    </view>
    <view class="row-actions">
     <button :aria-label="'绑定分类：'+group.name" @click="bindingId=group.id">绑定分类</button>
     <switch :key="group.id+'-'+switchRevision" :aria-label="group.name+'轮播开关'" :checked="group.enabled" @change="toggleGroup(group,checked($event))"/>
     <button class="expand-button" :aria-expanded="expandedId===group.id" :aria-controls="'carousel-content-'+group.id" @click="toggleExpanded(group.id)">{{expandedId===group.id?'收起 ▴':'展开 ▾'}}</button>
     <button class="danger" :aria-label="'删除轮播组：'+group.name" @click="removeGroup(group)">删除</button>
    </view>
   </view>
   <view v-if="expandedId===group.id" :id="'carousel-content-'+group.id" class="group-content">
   <view class="row content-heading"><view class="grow"><text class="section-title">轮播内容</text><text class="hint">每行一张轮播，修改自动保存。隐藏后保留内容，可随时开启。</text></view><button @click="addSlide(group)">＋ 添加轮播</button></view>
   <view v-if="!group.slides.length" class="empty small"><text class="hint">添加影片或上传图片，配置完成后再开启。</text></view>
   <view v-else class="table-scroll" role="region" :aria-label="group.name+'轮播内容，可横向滚动'" tabindex="0">
    <view class="slide-table" role="table" :aria-label="group.name+'轮播内容'">
     <view class="table-row table-head" role="row"><text role="columnheader">排序</text><text role="columnheader">轮播图片</text><text role="columnheader">展示标题</text><text role="columnheader">点击后跳转</text><text role="columnheader">显示开关</text><text role="columnheader">操作</text></view>
     <view v-for="(slide,index) in group.slides" :key="slide.id" class="table-row slide-row" role="row">
      <view class="table-cell order-cell" role="cell"><text class="number">{{String(index+1).padStart(2,'0')}}</text><view class="sort-actions"><button :disabled="index===0" :aria-label="'上移轮播 '+(index+1)" @click="move(group,index,-1)">↑</button><button :disabled="index===group.slides.length-1" :aria-label="'下移轮播 '+(index+1)" @click="move(group,index,1)">↓</button></view></view>
      <view class="table-cell image-cell" role="cell">
       <view class="image-tools"><view class="thumbnail"><image v-if="slide.image" :src="assetUrl(slide.image)" mode="aspectFit" :aria-label="'轮播 '+(index+1)+'图片'"/><text v-else>暂无图片</text></view><view class="image-actions"><button @click="openFilms(group,slide,'display')">CMS 选片</button><button :disabled="!!uploading" @click="upload(group,slide)">{{uploading===slide.id?'上传中…':'上传图片'}}</button></view></view>
       <input :aria-label="'轮播 '+(index+1)+'图片地址'" :value="slide.image" maxlength="2048" placeholder="图片地址 https://…" @blur="setImage(group,slide,($event as any).detail.value)"/>
       <text v-if="slide.sourceVodId" class="hint source-note">CMS 影片 #{{slide.sourceVodId}}</text>
      </view>
      <view class="table-cell title-cell" role="cell"><input :aria-label="'轮播 '+(index+1)+'展示标题'" :value="slide.title" maxlength="80" placeholder="填写标题（可选）" @input="setTitle(slide,($event as any).detail.value)"/></view>
      <view class="table-cell target-cell" role="cell">
       <EditorChoice :model-value="slide.target.type" :options="carouselTargetOptions" @update:model-value="changeTarget(group,slide,$event)"/>
       <view v-if="slide.target.type==='vod'" class="vod-target"><text class="hint">{{slide.target.vodId?'影片 #'+slide.target.vodId:'尚未选择影片'}}</text><button @click="openFilms(group,slide,'target')">选择影片</button></view>
       <view v-else-if="slide.target.type==='page'" class="page-target"><EditorChoice :model-value="slide.target.pageId" :options="pageOptions" @update:model-value="setPage(group,slide,$event)"/><text v-if="!theme.contentPages?.length" class="hint">暂无已创建页面</text></view>
       <input v-else-if="slide.target.type==='external'" :aria-label="'轮播 '+(index+1)+'第三方链接'" :value="slide.target.url" maxlength="2048" placeholder="https://example.com/activity" @blur="setExternal(group,slide,($event as any).detail.value)"/>
       <EditorChoice v-else :model-value="slide.target.page" :options="systemOptions" @update:model-value="slide.target={type:'system',page:$event as AppPage}"/>
      </view>
      <view class="table-cell visibility-cell" role="cell"><switch :key="slide.id+'-'+switchRevision" :aria-label="'轮播 '+(index+1)+'启用开关'" :checked="slide.enabled" @change="toggleSlide(group,slide,checked($event))"/><text class="visibility-label" :class="{enabled:slide.enabled}">{{slide.enabled?'已开启':'已隐藏'}}</text></view>
      <view class="table-cell action-cell" role="cell"><button class="danger" :aria-label="'删除轮播 '+(index+1)" @click="removeSlide(group,slide)">删除</button></view>
     </view>
    </view>
   </view>

   </view>
  </view>
 </view>
 <view v-if="bindingGroup" class="modal-backdrop" @click.self="bindingId=''">
  <view class="binding-dialog" role="dialog" aria-modal="true" :aria-label="'绑定一级分类：'+bindingGroup.name">
   <view class="row"><text class="section-title grow">{{bindingGroup.name}} · 绑定一级分类</text><button @click="bindingId=''">关闭</button></view>
   <text class="hint">每组绑定一个一级分类，每个分类只能绑定一组轮播。再次点击已选分类可解除绑定。</text>
   <view v-if="theme.categories.length" class="bindings">
    <button v-for="entry in theme.categories" :key="entry.id" :class="{chosen:bindingGroup.entryIds.includes(entry.id)}" :disabled="!!owner(entry.id)&&owner(entry.id)?.id!==bindingGroup.id" :aria-pressed="bindingGroup.entryIds.includes(entry.id)" @click="bind(bindingGroup,entry.id);bindingId=''">{{entry.label}}{{owner(entry.id)&&owner(entry.id)?.id!==bindingGroup.id?' · 已绑定其他组':''}}{{!entry.enabled?'（分类已关闭）':''}}</button>
   </view>
   <text v-else class="hint">请先在“导航与分类绑定 → 一级分类”中创建入口。</text>
  </view>
 </view>
 <view v-if="picker" class="modal-backdrop" @click.self="closeFilms"><view class="film-dialog" role="dialog" aria-modal="true" aria-label="选择 CMS 影片">
  <view class="row"><text class="section-title grow">{{picker.mode==='display'?'选择展示影片与图片':'选择点击后打开的影片'}}</text><button @click="closeFilms">关闭</button></view>
  <view class="search-bar"><input v-model="keyword" placeholder="搜索影片名称" @confirm="searchFilms(1)"/><EditorChoice v-model="categoryId" :options="categoryOptions"/><button :disabled="loading" @click="searchFilms(1)">搜索</button></view>
  <text v-if="loading" class="empty hint">正在读取苹果 CMS…</text><view v-else-if="error" class="empty"><text class="error">{{error}}</text><button @click="searchFilms(filmPage)">重试</button></view><text v-else-if="!films.length" class="empty hint">没有找到影片，请调整关键词或分类。</text>
  <view v-else class="film-grid"><button v-for="vod in films" :key="vod.id" class="film-card" @click="chooseFilm(vod)"><image :src="assetUrl(carouselFilmImage(vod))" mode="aspectFit"/><text class="film-title">{{vod.title}}</text><text class="hint">{{vod.category}} · #{{vod.id}}</text></button></view>
  <view class="row pagination"><text class="hint grow">共 {{total}} 部 · {{filmPage}} / {{filmPages}} 页</text><button :disabled="loading||filmPage<=1" @click="searchFilms(filmPage-1)">上一页</button><button :disabled="loading||filmPage>=filmPages" @click="searchFilms(filmPage+1)">下一页</button></view>
 </view></view>
</view>
</template>
<style scoped>
.carousel-center{padding:30px;color:var(--studio-text,#e4efea)}
.heading{display:flex;align-items:center;justify-content:space-between;gap:20px;margin-bottom:24px}
.title{display:block;font-size:22px;font-weight:650;margin-bottom:8px}
.hint{display:block;font-size:12px;line-height:1.7;color:var(--studio-muted,#91a9a0)}
button{margin:0;padding:5px 12px;font-size:12px;line-height:28px;border:1px solid var(--studio-border,#ffffff18);border-radius:7px;background:var(--studio-input,#172329);color:inherit;cursor:pointer}
button::after{border:0}button[disabled]{opacity:.35;cursor:default}
button:focus-visible,input:focus-visible,.table-scroll:focus-visible{outline:2px solid var(--studio-link,#c9eaa2);outline-offset:2px}
button.primary,.chosen{color:var(--studio-link,#c9eaa2);border-color:var(--studio-link,#c9eaa2)}
.danger{color:#f6a3a3}.row{display:flex;align-items:center;gap:8px;flex-wrap:wrap}.grow{flex:1;min-width:140px}
.section-title{display:block;font-size:14px;font-weight:600}.bindings{display:flex;gap:8px;flex-wrap:wrap;margin-top:14px}.content-heading{margin-top:10px}
.empty{padding:64px 20px;display:flex;flex-direction:column;align-items:center;gap:16px;text-align:center;border:1px dashed var(--studio-border,#ffffff18);border-radius:12px}.empty-title{font-size:18px}.empty.small{padding:30px}
input{height:38px;padding:0 10px;box-sizing:border-box;border:1px solid var(--studio-border,#ffffff18);border-radius:6px;background:var(--studio-input,#172329);color:var(--studio-text,#e4efea);font-size:13px;width:100%}
.table-scroll{max-width:100%;overflow-x:auto;border:1px solid var(--studio-border,#ffffff18);border-radius:9px}
.slide-table{min-width:880px;width:100%}.table-row{display:grid;grid-template-columns:64px minmax(220px,1.3fr) minmax(140px,.9fr) minmax(200px,1.2fr) 96px 68px}
.table-head{background:var(--studio-input,#172329);color:var(--studio-muted,#91a9a0);font-size:12px}
.table-head>text{padding:12px 10px;box-sizing:border-box}.table-head>text:first-child,.table-head>text:nth-last-child(-n+2){text-align:center}
.slide-row{border-top:1px solid var(--studio-border,#ffffff12)}.table-cell{min-width:0;padding:14px 10px;box-sizing:border-box;display:flex;flex-direction:column;gap:8px;justify-content:center}
.order-cell,.visibility-cell,.action-cell{align-items:center}.sort-actions{display:flex;gap:4px}.sort-actions button{padding:0;width:22px;line-height:26px}
.image-tools{display:flex;gap:10px;align-items:center}.thumbnail{width:108px;height:60px;flex-shrink:0;display:flex;align-items:center;justify-content:center;overflow:hidden;border:1px dashed var(--studio-border,#ffffff18);border-radius:5px;background:#0003;color:var(--studio-muted,#91a9a0);font-size:11px}.thumbnail image{width:100%;height:100%}
.image-actions{display:flex;flex-direction:column;gap:5px}.slide-table button{white-space:nowrap;line-height:24px;padding:2px 8px}.slide-table .sort-actions button{padding:0}.image-cell input{height:32px;font-size:12px}.source-note{font-size:10px}
.target-cell :deep(.choice){padding:8px 10px;min-height:34px}.vod-target{display:flex;align-items:center;justify-content:space-between;gap:6px}.page-target{display:flex;flex-direction:column;gap:4px}.target-cell input{height:34px;font-size:12px}.visibility-label{font-size:11px;color:var(--studio-muted,#91a9a0)}.visibility-label.enabled{color:var(--studio-link,#c9eaa2)}
.modal-backdrop{position:fixed;inset:0;background:#000a;z-index:1000;display:flex;align-items:center;justify-content:center;padding:24px}
.film-dialog{background:var(--studio-surface,#0f1b20);border:1px solid var(--studio-border,#ffffff18);border-radius:14px;width:960px;max-width:100%;max-height:85vh;overflow:auto;padding:24px;display:flex;flex-direction:column;gap:20px}
.search-bar{display:flex;align-items:center;gap:12px}.search-bar input{flex:1}.search-bar :deep(.choice){min-width:120px}
.film-grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(140px,1fr));gap:12px}.film-card{text-align:left;display:flex;flex-direction:column;padding:10px;gap:8px;white-space:normal}.film-card image{width:100%;height:130px;background:#0003}.film-title{font-size:13px;line-height:1.5;overflow-wrap:anywhere}.pagination{justify-content:flex-end}.error{color:#f6a3a3;font-size:12px}
@media(max-width:680px){.carousel-center{padding:16px}.heading{align-items:flex-start}.modal-backdrop{padding:10px}.film-dialog{padding:16px}.search-bar{flex-wrap:wrap}.search-bar input{flex-basis:100%}.film-grid{grid-template-columns:repeat(2,minmax(0,1fr))}}

.group-list{display:flex;flex-direction:column;border:1px solid var(--studio-border,#ffffff12);border-radius:12px;background:var(--studio-panel,#111b21)}
.group-module+.group-module{border-top:1px solid var(--studio-border,#ffffff12)}
.group-row{display:flex;align-items:center;gap:16px;min-height:90px;padding:18px 20px;box-sizing:border-box}
.number{font-size:11px;color:var(--studio-muted,#91a9a0);font-variant-numeric:tabular-nums;flex-shrink:0}
.group-copy{flex:1;min-width:120px}
.group-name-row{display:flex;align-items:center;min-height:32px}
.group-name-label{font-size:15px;font-weight:500;line-height:24px;overflow-wrap:anywhere;min-width:0}
button.rename-button{display:flex;align-items:center;justify-content:center;width:20px;height:22px;margin:0 0 0 2px;padding:0;flex-shrink:0;transform:translateY(-6px);font-size:12px;line-height:1;border:0;background:transparent}
button.rename-button:focus-visible{outline:1px solid var(--studio-text,#e4f1ea);outline-offset:1px}
input.rename-input{width:240px;max-width:100%;height:32px;box-sizing:border-box;font-size:15px}
.group-meta{display:block;font-size:11px;line-height:1.7;color:var(--studio-muted,#91a9a0);padding-left:4px;margin-top:4px}
.row-actions{display:flex;align-items:center;gap:10px;flex-shrink:0}
switch{transform:scale(.8);transform-origin:center}
.expand-button{min-width:76px}
.group-content{display:flex;flex-direction:column;gap:16px;padding:4px 24px 24px 52px;border-top:1px solid var(--studio-border,#ffffff12)}
.binding-dialog{width:560px;max-width:100%;max-height:80vh;overflow:auto;box-sizing:border-box;background:var(--studio-surface,#0f1b20);border:1px solid var(--studio-border,#ffffff18);border-radius:14px;padding:24px;display:flex;flex-direction:column;gap:16px}
.binding-dialog .bindings{margin-top:0}
@media(max-width:760px){.group-row{flex-wrap:wrap;gap:10px;padding:16px}.row-actions{width:100%;justify-content:flex-end;gap:6px}.group-content{padding:4px 16px 16px}.heading>.primary{white-space:nowrap}.heading{gap:12px}.row-actions button{padding:4px 9px}.binding-dialog{padding:18px}}
</style>
