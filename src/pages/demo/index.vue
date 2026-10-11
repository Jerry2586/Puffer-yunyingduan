<script setup lang="ts">
import {ref} from 'vue';
import {onLoad} from '@dcloudio/uni-app';
import CinemaDemo from '@/components/CinemaDemo.vue';
const device=ref<'mobile'|'tablet'|'web'>('web');
onLoad(options=>{if(['mobile','tablet','web'].includes(options?.device||''))device.value=options!.device as typeof device.value;});
</script>
<template><CinemaDemo :device="device"/></template>
