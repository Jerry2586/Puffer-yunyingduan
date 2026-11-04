<script setup lang="ts">
import {computed,reactive} from 'vue';
import type {Carousel,CarouselAppearance} from '@/domain/types';
import {carouselNumbers,carouselStyles,resolveCarouselAppearance} from '@/domain/carousel-appearance';
import type {CarouselNumber} from '@/domain/carousel-appearance';
import EditorChoice from './EditorChoice.vue';
const props=defineProps<{modelValue?:CarouselAppearance;base:Carousel}>();
const emit=defineEmits<{'update:modelValue':[value:CarouselAppearance|undefined]}>();
const geometry=computed(()=>resolveCarouselAppearance(props.base,props.modelValue));
const customized=computed(()=>Object.keys(props.modelValue||{}).length>0);
const drafts=reactive<Partial<Record<CarouselNumber,string>>>({});
function set<K extends keyof CarouselAppearance>(key:K,value:CarouselAppearance[K]){
 const next={...props.modelValue};if(value===undefined)delete next[key];else next[key]=value;
 emit('update:modelValue',Object.keys(next).length?next:undefined);
}
function number(key:CarouselNumber,event:any,commit=false){
 const raw=String(event.detail.value).trim();drafts[key]=raw;
 const rule=carouselNumbers.find(f=>f.key===key)!,value=Number(raw);
 if(commit){
  delete drafts[key];
  if(!raw){set(key,undefined);return;}
  if(Number.isFinite(value))set(key,Math.max(rule.min,Math.min(rule.max,Math.round(value))));
 }else if(!raw)set(key,undefined);
 else if(Number.isInteger(value)&&value>=rule.min&&value<=rule.max)set(key,value);
}
function reset(){for(const field of carouselNumbers)delete drafts[field.key];emit('update:modelValue',undefined);}
</script>
<template><view class="carousel-settings">
 <view class="settings-heading"><view><text class="settings-title">轮播样式</text><text class="settings-hint">仅影响当前页面，修改后自动保存。未调整的设置沿用主题默认。</text></view><button class="reset" :disabled="!customized" @click="reset">恢复主题默认</button></view>
 <label>排版样式<EditorChoice :model-value="geometry.style" :options="carouselStyles" @update:model-value="set('style',String($event) as CarouselAppearance['style'])"/></label>
 <view class="number-grid"><label v-for="field in carouselNumbers" :key="field.key">{{field.label}}（{{field.unit}}）<input type="number" :aria-label="'轮播'+field.label.replace('轮播','')" :value="drafts[field.key]??geometry[field.key]" @input="number(field.key,$event)" @blur="number(field.key,$event,true)" @confirm="number(field.key,$event,true)"/><text class="range">{{field.min}}–{{field.max}} {{field.unit}}</text></label></view>
 <label>图片显示<EditorChoice :model-value="geometry.fit" :options="[{id:'cover',label:'铺满裁切'},{id:'contain',label:'完整显示'}]" @update:model-value="set('fit',String($event) as CarouselAppearance['fit'])"/></label>
</view></template>
<style scoped>
.carousel-settings{display:flex;flex-direction:column;gap:20px;padding:20px;border:1px solid var(--studio-border,#ffffff12);border-radius:12px;background:var(--studio-panel,#111b21);color:var(--studio-text,#d8e3df)}
.settings-heading{display:flex;align-items:flex-start;justify-content:space-between;gap:16px}.settings-title{display:block;font-size:13px;font-weight:600}.settings-hint{display:block;font-size:11px;line-height:1.8;color:var(--studio-muted,#91a9a0);margin-top:5px}.number-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:16px 24px}label{display:flex;flex-direction:column;gap:8px;min-width:0;font-size:11px}.range{font-size:10px;color:var(--studio-muted,#91a9a0)}input{height:40px;box-sizing:border-box;padding:0 11px;border:1px solid var(--studio-border,#ffffff12);border-radius:6px;background:var(--studio-input,#172329);color:inherit;font-size:12px;font-variant-numeric:tabular-nums}.reset{flex-shrink:0;margin:0;padding:4px 12px;border:1px solid var(--studio-border,#ffffff12);border-radius:6px;background:var(--studio-input,#172329);color:inherit;font-size:11px;line-height:28px;cursor:pointer}.reset::after{border:0}.reset[disabled]{opacity:.4;cursor:default}input:focus-visible,.reset:focus-visible{outline:2px solid var(--studio-link,#c9eaa2);outline-offset:3px}@media(max-width:520px){.carousel-settings{padding:16px}.settings-heading{flex-direction:column;gap:10px}.number-grid{grid-template-columns:1fr}}
</style>
