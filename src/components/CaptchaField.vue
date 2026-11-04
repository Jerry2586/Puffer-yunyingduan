<script setup lang="ts">
import {ref,watch,onBeforeUnmount} from 'vue';
import {applicationApi,type CaptchaInput,type CaptchaChallenge} from '@/services/application';
import {applicationRuntime} from '@/services/application-runtime';
const props=defineProps<{action:'login'|'register'|'comments'|'danmu';modelValue:CaptchaInput;disabled?:boolean;demo?:boolean}>();
const emit=defineEmits<{'update:modelValue':[value:CaptchaInput];ready:[value:boolean]}>();
const challenge=ref<CaptchaChallenge>({required:false}),answer=ref(''),loading=ref(false),error=ref('');let generation=0;
async function refresh(){const token=++generation;challenge.value={required:false};answer.value='';emit('update:modelValue',{});emit('ready',false);error.value='';
 if(props.demo){loading.value=false;emit('ready',true);return;}
 loading.value=true;try{const value=await applicationApi.captcha(props.action);if(token!==generation)return;challenge.value=value;emit('update:modelValue',value.required?{captchaToken:value.token,captchaAnswer:''}:{});emit('ready',true);}catch(e){if(token===generation)error.value=(e as Error).message;}finally{if(token===generation)loading.value=false;}
}
watch(answer,value=>{if(!loading.value)emit('update:modelValue',challenge.value.required?{captchaToken:challenge.value.token,captchaAnswer:value}:{});},{flush:'sync'});
onBeforeUnmount(()=>generation++);
watch(()=>[props.action,props.demo,applicationRuntime.version],()=>void refresh(),{immediate:true});defineExpose({refresh});
</script>
<template><view v-if="challenge.required||loading||error" class="captcha-field"><view v-if="challenge.required" class="captcha-row"><input v-model="answer" aria-label="图形验证码" placeholder="图形验证码" maxlength="8" :disabled="disabled||loading"/><image :src="challenge.image" mode="aspectFit" role="button" aria-label="点击更换验证码" @click="refresh"/></view><text v-if="loading">正在读取验证码…</text><text v-if="error" class="captcha-error" role="button" @click="refresh">{{error}} · 点击重试</text><text v-else-if="challenge.required" role="button" @click="refresh">看不清，换一张</text></view></template>
<style scoped>.captcha-field{display:flex;flex-direction:column;gap:8px;font-size:12px;color:#9298a3}.captcha-row{display:flex;align-items:center;gap:12px}.captcha-row input{flex:1;min-width:0;height:40px;border:1px solid #80808040;border-radius:6px;padding:0 12px}.captcha-row image{width:120px;height:42px;background:#fff;border-radius:6px;cursor:pointer}.captcha-error{color:#e99b91;cursor:pointer}</style>
