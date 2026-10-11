<script setup lang="ts">
import {computed} from 'vue';
const props=withDefaults(defineProps<{color?:string;size?:number;framed?:boolean}>(),{color:'#ccf279',size:40,framed:false});
const dimension=computed(()=>Math.max(24,Math.min(160,props.size))+'px');
const artwork=computed(()=>{
  const color=/^#[0-9a-f]{3,8}$/i.test(props.color)?props.color:'#ccf279';
  const fill=props.framed?'url(#metal)':color;
  const svg='<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 120 120"><defs><linearGradient id="metal" x1="0" y1="0" x2="0.85" y2="1"><stop stop-color="#f4e5c4"/><stop offset="0.48" stop-color="#d5b77b"/><stop offset="1" stop-color="#a78149"/></linearGradient></defs><path fill="'+fill+'" fill-rule="evenodd" d="M35 48C39 34 50 25 63 23L70 15L75 25C93 29 104 44 103 61C103 66 101 72 99 77L105 84L94 85C85 96 69 100 55 95L46 101L45 89C40 83 36 77 35 69L19 78Q16 79 18 75L24 59L18 43Q16 39 20 41L35 48ZM58 46Q55 44 55 48V73Q55 77 58 75L78 62Q82 60 78 58L58 46ZM85 40a2.6 2.6 0 1 0 0 5.2a2.6 2.6 0 1 0 0-5.2Z"/></svg>';
  return 'data:image/svg+xml;charset=utf-8,'+encodeURIComponent(svg);
});
</script>

<template>
  <view class="puffer" :class="{'puffer-framed':framed}" :style="{width:dimension,height:dimension}" aria-label="河豚影视标识" role="img">
    <image class="puffer-art" :src="artwork" mode="aspectFit"/>
  </view>
</template>

<style scoped>
.puffer{display:inline-flex;position:relative;align-items:center;justify-content:center;flex-shrink:0;vertical-align:middle}
.puffer-art{width:100%;height:100%;display:block}
.puffer-framed{box-sizing:border-box;border:1px solid #d5b77b42;border-radius:27%;background:linear-gradient(145deg,#29302f,#151e21 65%);box-shadow:inset 0 1px 0 #fff1,0 5px 15px #0002}
.puffer-framed .puffer-art{width:88%;height:88%}
</style>
