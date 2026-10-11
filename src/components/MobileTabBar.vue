<script setup lang="ts">
import type {Theme,NavItem,AppPage} from '@/domain/types';
import {assetUrl} from '@/services/api';
import AppIcon from './AppIcon.vue';
const props=defineProps<{theme:Theme;items:NavItem[];page:AppPage;editing?:boolean}>();
const emit=defineEmits<{navigate:[item:NavItem];select:[]}>();
const stockSymbols=new Set(['','⌂','▷','♔','○','◉','▦','☆','↗','▤','⚙','⌕','▶','♡','♥']);
const iconNames:Record<string,string>={home:'home',shortdrama:'shortdrama',member:'member',mine:'mine',category:'category',search:'search',ranking:'ranking',schedule:'schedule',history:'history',library:'favorite',settings:'settings'};
function artwork(n:NavItem){return props.page===n.page?(n.activeIcon||n.icon||''):(n.icon||'');}
function customImage(n:NavItem){return artwork(n).startsWith('http://')||artwork(n).startsWith('https://')||artwork(n).startsWith('/api/');}
function choose(n:NavItem){if(props.editing)emit('select');else emit('navigate',n);}
</script>
<template>
 <view class="mobile-tabbar" :class="['nav-'+theme.pageStyles.navigation,{'youku-nav':theme.pageStyles.design==='youku'}]" role="navigation" aria-label="底部导航">
  <button v-for="n in items" :key="n.id" class="nav-item" :class="{chosen:page===n.page}" :aria-label="n.label" :aria-current="page===n.page?'page':undefined" @click="choose(n)">
   <view class="nav-art">
    <image v-if="customImage(n)" :src="assetUrl(artwork(n))" mode="aspectFit"/>
    <AppIcon v-else-if="stockSymbols.has(artwork(n))" :name="iconNames[n.page]||'category'" :active="page===n.page" :size="23" :color="page===n.page?theme.colors.accent:theme.colors.muted"/>
    <text v-else class="custom-glyph">{{artwork(n)}}</text>
   </view>
   <text class="nav-label">{{n.label}}</text><view class="nav-marker"/>
  </button>
 </view>
</template>
<style scoped>
.mobile-tabbar{display:flex;align-items:stretch;flex-shrink:0;position:relative;z-index:20;background:var(--mobile-nav,var(--bg));border-top:1px solid var(--mobile-line,#80808018);padding:6px 8px calc(7px + env(safe-area-inset-bottom));box-shadow:0 -6px 24px #00000004}
.mobile-tabbar .nav-item{position:relative;flex:1;min-width:0;min-height:51px;margin:0;padding:4px 2px 2px;display:flex;flex-direction:column;justify-content:center;align-items:center;gap:4px;background:transparent;border:0;border-radius:10px;color:var(--muted);line-height:1.3;font-family:inherit;font-size:11px;font-weight:500;box-sizing:border-box;cursor:pointer;transition:color .18s,opacity .18s;-webkit-tap-highlight-color:transparent}
.nav-item::after{border:0}.nav-item:focus{outline:none}.nav-item:focus-visible{outline:2px solid var(--accent);outline-offset:-2px}.nav-item:active{opacity:.65}.mobile-tabbar .nav-item.chosen{color:var(--text)!important;font-weight:650}.nav-art{height:24px;width:30px;display:flex;align-items:center;justify-content:center}.nav-art image{width:23px;height:23px}.custom-glyph{font-size:23px;line-height:24px}.nav-label{max-width:100%;overflow:hidden;white-space:nowrap;text-overflow:ellipsis;letter-spacing:.2px}.nav-marker{position:absolute;width:4px;height:4px;border-radius:50%;background:var(--accent);top:-3px;opacity:0}.chosen .nav-marker{opacity:1}.nav-text .nav-art{display:none}.nav-text .nav-item{font-size:14px;min-height:43px;font-weight:500}.nav-text .chosen{font-weight:700}.nav-text .nav-marker{top:auto;bottom:1px;width:14px;height:3px;border-radius:3px}
.youku-nav{padding:0 3px env(safe-area-inset-bottom);box-shadow:none;border-top:1px solid var(--mobile-line);background:var(--mobile-nav)}.youku-nav .nav-marker{display:none}.youku-nav.nav-text .nav-item{min-height:39px;font-size:12px;font-weight:400;border-radius:0;padding:7px 1px;gap:0}.youku-nav.nav-text .nav-item.chosen{font-weight:400}.youku-nav .nav-label{letter-spacing:0}</style>
