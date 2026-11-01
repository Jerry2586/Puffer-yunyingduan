<script setup lang="ts">
import {computed} from 'vue';
import type {Theme} from '@/domain/types';
import {navigationArtwork,navigationIcon} from '@/domain/operation-pages';
import {assetUrl} from '@/services/api';
import AppIcon from './AppIcon.vue';
const props=withDefaults(defineProps<{theme:Theme;entry:{id:string;page?:string;iconKey?:string;icon?:string;activeIcon?:string};active?:boolean;size?:number;color?:string}>(),{active:false,size:23});
const artwork=computed(()=>navigationArtwork(props.theme,props.entry,props.active));
const image=computed(()=>/^(https?:\/\/|\/api\/)/.test(artwork.value));
const stock=new Set(['','⌂','▷','♔','○','◉','▦','☆','↗','▤','⚙','⌕','▶','♡','♥']);
const builtins=new Set(['home','shortdrama','member','mine','category','search','ranking','schedule','history','favorite','settings','shield','info']);
const name=computed(()=>builtins.has(artwork.value)?artwork.value:navigationIcon(props.entry.page||({all:'category'} as Record<string,string>)[props.entry.id]||props.entry.id));
</script>
<template><view class="navigation-artwork" :style="{width:size+'px',height:size+'px',color}" aria-hidden="true"><image v-if="image" :src="assetUrl(artwork)" mode="aspectFit"/><AppIcon v-else-if="stock.has(artwork)||builtins.has(artwork)" :name="name" :active="active" :size="size" :color="color||theme.colors.text"/><text v-else :style="{fontSize:size+'px'}">{{artwork}}</text></view></template>
<style scoped>.navigation-artwork{display:inline-flex;align-items:center;justify-content:center;flex-shrink:0;line-height:1}.navigation-artwork image{width:100%;height:100%}.navigation-artwork text{line-height:1}</style>
