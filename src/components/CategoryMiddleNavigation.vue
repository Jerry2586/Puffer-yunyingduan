<script setup lang="ts">
import type {CategoryShortcut,CategoryShortcutLayout,Theme} from '@/domain/types';
import NavigationArtwork from './NavigationArtwork.vue';
withDefaults(defineProps<{theme:Theme;items:CategoryShortcut[];layout:CategoryShortcutLayout;columns:number;active?:string}>(),{active:''});
const emit=defineEmits<{select:[id:CategoryShortcut['id']]}>();
</script>
<template><view v-if="items.length" class="category-middle-nav" role="navigation" aria-label="分类中间导航">
 <scroll-view :scroll-x="layout==='scroll'" :show-scrollbar="false">
  <view class="entries" :class="layout" :style="{'--columns':columns}">
   <button v-for="n in items" :key="n.id" :class="{active:n.id===active}" :aria-label="n.label" :aria-current="n.id===active?'page':undefined" @click="emit('select',n.id)">
    <NavigationArtwork v-if="n.display!=='text'" :theme="theme" :entry="n" :active="n.id===active" :size="36" :color="n.id===active?theme.colors.accent:theme.colors.text"/>
    <text v-if="n.display!=='icon'">{{n.label}}</text>
   </button>
  </view>
 </scroll-view>
</view></template>
<style scoped>
.category-middle-nav{width:100%;min-width:0;flex-shrink:0;color:var(--muted)}
.entries{gap:8px;padding:12px;box-sizing:border-box}
.entries.scroll{display:flex;min-width:100%;width:max-content}
.entries.grid{display:grid;grid-template-columns:repeat(var(--columns),minmax(0,1fr));row-gap:16px}
.entries button{display:flex;flex-direction:column;align-items:center;justify-content:center;gap:8px;min-width:0;min-height:64px;margin:0;padding:4px;border:0;border-radius:8px;background:transparent;color:inherit;font-size:12px;line-height:1.4;cursor:pointer}
.scroll button{flex:1 0 72px;max-width:112px}
.entries button::after{border:0}.entries button.active{color:var(--accent)}
.entries button:focus-visible{outline:2px solid var(--accent);outline-offset:-2px}
.entries text{max-width:100%;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
</style>
