<script setup lang="ts">
import {ref} from 'vue';
import type {LayoutAppearance} from '@/domain/types';
import {layoutNumbers,layoutColors,layoutValue,layoutColor} from '@/domain/operation-layout';
import type {LayoutKind} from '@/domain/operation-layout';
import EntryTextColor from './EntryTextColor.vue';
import EditorChoice from './EditorChoice.vue';
const props=defineProps<{modelValue?:LayoutAppearance;kind:LayoutKind;title:string;textColor:string;background:string;accent:string}>();
const emit=defineEmits<{ 'update:modelValue':[value:LayoutAppearance|undefined] }>();
const open=ref(false);
function set(key:string,value:string|number|undefined){const next={...props.modelValue};if(value===undefined)delete next[key];else next[key]=value;emit('update:modelValue',Object.keys(next).length?next:undefined);}
function number(key:string,event:any){const rule=layoutNumbers[props.kind].find(n=>n.key===key)!;const raw=String(event.detail.value).trim();if(!raw){set(key,undefined);return;}const value=Number(raw);if(!Number.isFinite(value))return;set(key,Math.max(rule.min,Math.min(rule.max,Math.round(value))));}
function fallback(key:string){return key.toLowerCase().includes('background')?props.background:key==='activeColor'?props.accent:props.textColor;}
</script>
<template><view class="layout-fields">
 <button class="disclosure" :aria-expanded="open" @click="open=!open"><text>{{title}}</text><text>{{open?'收起':'尺寸与样式 ›'}}</text></button>
 <view v-if="open" class="fields-body">
  <text class="hint">仅影响{{kind==='section'?'当前栏目':kind==='middle'?'当前一级分类的中间导航':'左上角菜单'}}。修改后自动保存；颜色可恢复主题默认。</text>
  <label v-if="kind==='middle'">图文排列<EditorChoice :model-value="modelValue?.arrangement||'column'" :options="[{id:'column',label:'图上文下'},{id:'row',label:'图左文右'}]" @update:model-value="set('arrangement',String($event))"/></label>
  <view class="number-grid"><label v-for="field in layoutNumbers[kind]" :key="field.key">{{field.label}}（{{field.unit||'px'}}）<input type="number" :aria-label="title+' '+field.label" :value="layoutValue(kind,modelValue,field.key)" @blur="number(field.key,$event)" @confirm="number(field.key,$event)"/><text class="range">{{field.min}}–{{field.max}}{{field.unit||'px'}}</text></label></view>
  <view class="color-row" v-for="field in layoutColors[kind]" :key="field.key"><text>{{field.label}}</text><EntryTextColor :label="title+' '+field.label" :model-value="typeof modelValue?.[field.key]==='string'?String(modelValue[field.key]):undefined" :default-color="layoutColor(modelValue,field.key,fallback(field.key))" @update:model-value="set(field.key,$event)"/></view>
  <button class="reset" :disabled="!modelValue" @click="emit('update:modelValue',undefined)">恢复默认尺寸与颜色</button>
 </view>
</view></template>
<style scoped>
.layout-fields{border:1px solid var(--studio-border,#ffffff18);border-radius:10px;overflow:hidden;min-width:0}.disclosure{display:flex;align-items:center;justify-content:space-between;gap:12px;width:100%;padding:12px 14px;border:0;background:var(--studio-input,#172329);color:var(--studio-text,#d8e3df);font-size:12px;text-align:left;line-height:1.6;margin:0;cursor:pointer}.disclosure>text:last-child{font-size:11px;color:var(--studio-muted,#91a9a0)}button::after{border:0}.fields-body{padding:16px;display:flex;flex-direction:column;gap:16px}.hint,.range{font-size:11px;line-height:1.6;color:var(--studio-muted,#91a9a0)}.number-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:16px}label{display:flex;flex-direction:column;gap:7px;font-size:11px;color:var(--studio-text,#d8e3df)}input{box-sizing:border-box;height:38px;width:100%;padding:0 10px;background:var(--studio-input,#172329);border:1px solid var(--studio-border,#ffffff18);border-radius:6px;color:inherit;font-size:12px}.color-row{display:flex;justify-content:space-between;align-items:center;font-size:12px;color:var(--studio-text,#d8e3df)}.reset{margin:0;padding:5px 12px;font-size:11px;background:transparent;color:var(--studio-link,#c9eaa2);border:1px solid var(--studio-border,#ffffff18);border-radius:6px;cursor:pointer}.reset[disabled]{opacity:.4;cursor:default}button:focus-visible,input:focus-visible{outline:2px solid var(--studio-link,#c9eaa2);outline-offset:2px}@media(max-width:520px){.number-grid{grid-template-columns:1fr}}
</style>
