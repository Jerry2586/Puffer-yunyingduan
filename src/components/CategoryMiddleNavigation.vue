<script setup lang="ts">
import {computed} from 'vue';
import {layoutValue,layoutColor} from '@/domain/operation-layout';
import type {LayoutAppearance,CategoryShortcut,CategoryShortcutLayout,Theme} from '@/domain/types';
import NavigationArtwork from './NavigationArtwork.vue';
const props=withDefaults(defineProps<{appearance?:LayoutAppearance;theme:Theme;items:CategoryShortcut[];layout:CategoryShortcutLayout;columns:number;active?:string}>(),{active:''});
const size=computed(()=>layoutValue('middle',props.appearance,'iconSize'));
const color=(active:boolean)=>layoutColor(props.appearance,active?'activeColor':'color',active?props.theme.colors.accent:props.theme.colors.text);
const vars=computed(()=>{const a=props.appearance,n=(k:string)=>layoutValue('middle',a,k);return {'--middle-font':n('fontSize')+'px','--middle-gap':n('gap')+'px','--middle-content-gap':n('contentGap')+'px','--middle-padding':n('padding')+'px','--middle-height':n('itemHeight')+'px','--middle-flex':a?.itemWidth!==undefined?'0 0 '+n('itemWidth')+'px':'1 0 72px','--middle-max-width':a?.itemWidth!==undefined?'none':'112px','--middle-row-gap':(a?.gap!==undefined?n('gap'):16)+'px','--middle-radius':n('radius')+'px','--middle-direction':a?.arrangement==='row'?'row':'column','--middle-color':layoutColor(a,'color',props.theme.colors.muted),'--middle-active-color':color(true),'--middle-active-bg':layoutColor(a,'activeBackground','transparent'),background:layoutColor(a,'background','transparent')};});
const emit=defineEmits<{select:[id:CategoryShortcut['id']]}>();
</script>
<template><view v-if="items.length" class="category-middle-nav" :style="vars" role="navigation" aria-label="分类中间导航">
 <scroll-view :scroll-x="layout==='scroll'" :show-scrollbar="false">
  <view class="entries" :class="layout" :style="{'--columns':columns}">
   <button v-for="n in items" :key="n.id" :class="{active:n.id===active}" :aria-label="n.label" :aria-current="n.id===active?'page':undefined" @click="emit('select',n.id)">
    <NavigationArtwork v-if="n.display!=='text'" :theme="theme" :entry="n" :active="n.id===active" :size="size" :color="color(n.id===active)"/>
    <text v-if="n.display!=='icon'">{{n.label}}</text>
   </button>
  </view>
 </scroll-view>
</view></template>
<style scoped>
.category-middle-nav{width:100%;min-width:0;flex-shrink:0;color:var(--middle-color)}
.entries{gap:var(--middle-gap);padding:var(--middle-padding);box-sizing:border-box}
.entries.scroll{display:flex;min-width:100%;width:max-content}
.entries.grid{display:grid;grid-template-columns:repeat(var(--columns),minmax(0,1fr));row-gap:var(--middle-row-gap)}
.entries button{display:flex;flex-direction:var(--middle-direction);align-items:center;justify-content:center;gap:var(--middle-content-gap);min-width:0;min-height:var(--middle-height);margin:0;padding:4px;border:0;border-radius:var(--middle-radius);background:transparent;color:inherit;font-size:var(--middle-font);line-height:1.4;cursor:pointer}
.scroll button{flex:var(--middle-flex);max-width:var(--middle-max-width)}
.entries button::after{border:0}.entries button.active{color:var(--middle-active-color);background:var(--middle-active-bg)}
.entries button:focus-visible{outline:2px solid var(--accent);outline-offset:-2px}
.entries text{max-width:100%;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
</style>
