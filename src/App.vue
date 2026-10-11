<script setup lang="ts">
import { onLaunch } from '@dcloudio/uni-app';
onLaunch(() => {});
</script>
<style>
page { background:#0b1115; color:#f3f5f0; font-family: -apple-system,BlinkMacSystemFont,"Segoe UI","PingFang SC","Microsoft YaHei",sans-serif; }
view,text,input,button,scroll-view {box-sizing:border-box;}
button { margin:0; font-size:14px; line-height:1.5; } button::after {border:0;}
input {font-family:inherit;}

/* #ifdef H5 */
html,body,uni-page-body{scrollbar-width:auto;scrollbar-color:auto}
html::-webkit-scrollbar,body::-webkit-scrollbar,uni-page-body::-webkit-scrollbar{width:6px;height:6px}
html::-webkit-scrollbar-thumb,body::-webkit-scrollbar-thumb,uni-page-body::-webkit-scrollbar-thumb{background:#63757e;border-radius:12px}
html::-webkit-scrollbar-track,body::-webkit-scrollbar-track,uni-page-body::-webkit-scrollbar-track{background:transparent}
html::-webkit-scrollbar-button,body::-webkit-scrollbar-button,uni-page-body::-webkit-scrollbar-button{display:none;width:0;height:0}
@supports not selector(::-webkit-scrollbar){html,body,uni-page-body{scrollbar-width:thin;scrollbar-color:#63757e transparent}}
/* #endif */
</style>
