<script setup lang="ts">
import {computed,ref,watch} from 'vue';
import type {Category,Theme} from '@/domain/types';
import {categoryShortcutGroup,categoryShortcuts,categoryShortcutSettings,visibleCategoryShortcuts} from '@/domain/operation-pages';
import EditorChoice from './EditorChoice.vue';
const props=defineProps<{theme:Theme;categories:Category[]}>();
type DisplayMode='text'|'both';
const bindingId=ref('');
const roots=computed(()=>props.theme.categories);
const categoryRows=computed(()=>roots.value.filter(entry=>categoryShortcutGroup(props.theme,entry.id)).map(entry=>({entry,settings:categoryShortcutSettings(props.theme,entry.id),count:visibleCategoryShortcuts(props.theme,entry.id).length,display:displayFor(entry.id)})));
const availableCategories=computed(()=>roots.value.filter(entry=>!categoryShortcutGroup(props.theme,entry.id)).map(entry=>({id:entry.id,label:entry.label})));
watch(()=>availableCategories.value.map(entry=>entry.id),ids=>{if(!ids.includes(bindingId.value))bindingId.value=ids[0]||'';},{immediate:true});
function displayFor(entryId:string):DisplayMode{const rows=categoryShortcuts(props.theme,entryId);return rows.length&&rows.every(n=>n.display==='text')?'text':'both';}
function editable(entryId:string){const group=categoryShortcutGroup(props.theme,entryId);if(group){group.entryId=entryId;delete group.categoryId;}return group;}
function addBinding(){const entryId=bindingId.value;if(!roots.value.some(entry=>entry.id===entryId)||categoryShortcutGroup(props.theme,entryId))return;props.theme.categoryShortcuts??=[];props.theme.categoryShortcuts.push({entryId,enabled:false,layout:'scroll',columns:4,items:categoryShortcuts(props.theme,entryId).map(n=>({...n,display:'both',enabled:false}))});}
function toggleCategory(entryId:string,enabled:boolean){if(!roots.value.some(entry=>entry.id===entryId))return;const group=editable(entryId);if(group)group.enabled=enabled;}
function setDisplay(entryId:string,mode:DisplayMode){if(!roots.value.some(entry=>entry.id===entryId))return;const group=editable(entryId);if(group)for(const item of group.items)item.display=mode;}
function resourceLabel(id:string){return id?(props.categories.find(category=>category.id===id)?.name||'CMS #'+id):'未绑定 CMS 分类';}
</script>
<template><view class="shortcut-editor">
 <view class="category-heading"><text class="setting-title">一级分类中间导航</text><text class="hint">添加并绑定一级分类，分别控制中间导航的显示开关及文字／图文样式。</text></view>
 <view v-if="availableCategories.length" class="binding-bar"><label>绑定一级分类<EditorChoice v-model="bindingId" :options="availableCategories"/></label><button :disabled="!bindingId" @click="addBinding">添加并绑定</button></view>
 <text v-else-if="roots.length" class="hint">已有一级分类均已添加。</text>
 <view v-if="categoryRows.length" class="category-list">
  <view v-for="row in categoryRows" :key="row.entry.id" class="category-row">
   <view class="category-copy"><text class="category-name">{{row.entry.label}}</text><text class="hint">{{resourceLabel(row.entry.categoryId)}}{{row.entry.categoryId?'及其下级分类':''}}</text></view>
   <view class="category-actions"><view class="display-modes" :aria-label="row.entry.label+'导航样式'"><button :aria-pressed="row.display==='text'" :class="{chosen:row.display==='text'}" @click="setDisplay(row.entry.id,'text')">文字导航</button><button :aria-pressed="row.display==='both'" :class="{chosen:row.display==='both'}" @click="setDisplay(row.entry.id,'both')">图文导航</button></view><text class="category-state" :class="{enabled:row.settings.enabled}">{{row.settings.enabled?'已开启 · '+row.count+' 个入口':'已关闭'}}</text><switch :aria-label="row.entry.label+'中间导航'" :checked="row.settings.enabled" @change="toggleCategory(row.entry.id,Boolean(($event as any).detail.value))"/></view>
  </view>
 </view>
 <text v-if="!categoryRows.length" class="hint empty">{{roots.length?'请选择一级分类，添加并绑定其中间导航。':'请先在“一级分类”中创建 APP 入口，再添加中间导航。'}}</text>
</view></template>
<style scoped>
.shortcut-editor{display:flex;flex-direction:column;gap:16px}.category-heading{display:flex;flex-direction:column;gap:4px}.binding-bar{display:flex;align-items:flex-end;gap:12px}.binding-bar label{flex:1;max-width:360px}.category-list{border:1px solid var(--studio-border,#ffffff12);border-radius:10px;overflow:hidden}.category-row{display:flex;align-items:center;justify-content:space-between;gap:16px;padding:14px 16px;border-bottom:1px solid var(--studio-border,#ffffff12)}.category-row:last-child{border-bottom:0}.category-copy{min-width:0;flex:1}.category-name{display:block;font-size:13px;overflow-wrap:anywhere}.category-actions{display:flex;align-items:center;gap:12px;flex-shrink:0}.display-modes{display:flex;gap:4px}.display-modes .chosen{border-color:var(--studio-link,#c9eaa2);color:var(--studio-link,#c9eaa2)}.category-state{font-size:11px;color:var(--studio-muted,#91a9a0)}.category-state.enabled{color:var(--studio-link,#c9eaa2)}.hint{display:block;color:var(--studio-muted,#91a9a0);font-size:11px;line-height:1.7}.setting-title{font-size:13px}label{display:flex;flex-direction:column;gap:8px;font-size:11px}button{margin:0;padding:5px 12px;border:1px solid var(--studio-border,#ffffff12);border-radius:6px;background:var(--studio-input,#172329);color:inherit;font-size:11px;line-height:28px;cursor:pointer}button::after{border:0}button:focus-visible{outline:2px solid var(--studio-link,#c9eaa2);outline-offset:2px}button[disabled]{opacity:.35}.empty{padding:24px}switch{transform:scale(.8)}@media(max-width:1080px){.category-row{flex-wrap:wrap;gap:8px}.category-actions{width:100%;justify-content:flex-end}}@media(max-width:600px){.category-actions{flex-wrap:wrap;gap:8px}.display-modes{margin-right:auto}.category-state{min-width:100px}.binding-bar{align-items:stretch;flex-direction:column}.binding-bar label{max-width:none}}
</style>
