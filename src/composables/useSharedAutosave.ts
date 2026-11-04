import {ref,computed,watch,onBeforeUnmount,type Ref} from 'vue';
const copy=<T,>(value:T):T=>JSON.parse(JSON.stringify(value));
// A single writer retains newer typing while a previous snapshot is in flight.
export function useSharedAutosave<T extends {version:number}>(options:{get:()=>Promise<T>;save:(snapshot:T)=>Promise<T>;validate:(snapshot:T)=>string;signature?:(snapshot:T)=>string}) {
 const draft=ref(null) as Ref<T|null>,saved=ref(null) as Ref<T|null>;
 const loading=ref(false),saving=ref(false),error=ref(''),conflict=ref(false);
 const signature=options.signature||((v:T)=>JSON.stringify(v));
 const dirty=computed(()=>!!draft.value&&!!saved.value&&signature(draft.value)!==signature(saved.value));
 let timer:ReturnType<typeof setTimeout>|undefined,flight:Promise<boolean>|null=null,alive=true,hydrating=false;
 const status=computed(()=>loading.value?'正在读取':conflict.value?'配置冲突':error.value?'保存失败':saving.value?'正在保存':dirty.value?'等待保存':'已自动保存');
 function schedule(){if(timer)clearTimeout(timer);if(!alive||hydrating||conflict.value||!dirty.value)return;timer=setTimeout(()=>{timer=undefined;void flush();},650);}
 watch(draft,schedule,{deep:true,flush:'sync'});
 async function load(){if(loading.value||saving.value)return;loading.value=true;error.value='';try{const v=await options.get();if(!alive)return;hydrating=true;saved.value=copy(v);draft.value=copy(v);conflict.value=false;}catch(e){if(alive)error.value=(e as Error).message;}finally{hydrating=false;if(alive)loading.value=false;}}
 async function flush():Promise<boolean>{
  if(timer){clearTimeout(timer);timer=undefined;}if(flight){await flight;return error.value?false:flush();}
  if(!alive||!draft.value||!saved.value)return false;if(!dirty.value)return true;if(conflict.value)return false;
  const snapshot=copy(draft.value),invalid=options.validate(snapshot);if(invalid){error.value=invalid;return false;}
  snapshot.version=saved.value.version;saving.value=true;error.value='';
  flight=(async()=>{try{const v=await options.save(snapshot);if(!alive)return true;hydrating=true;const unchanged=!!draft.value&&signature(draft.value)===signature(snapshot);saved.value=copy(v);if(unchanged)draft.value=copy(v);else if(draft.value)draft.value.version=v.version;return true;}catch(e){if(alive){error.value=(e as Error).message;conflict.value=(e as any)?.status===409;}return false;}finally{hydrating=false;saving.value=false;}})();
  const ok=await flight;flight=null;if(ok&&alive&&dirty.value)return flush();return ok;
 }
 async function reload(){if(dirty.value){const discard=await new Promise<boolean>(resolve=>uni.showModal({title:'读取服务端配置',content:'当前修改尚未保存，重新读取会放弃这些修改。',success:r=>resolve(!!r.confirm),fail:()=>resolve(false)}));if(!discard)return;}await load();}
 async function prepareToLeave(){if(!dirty.value&&!saving.value)return true;if(await flush())return true;const discard=await new Promise<boolean>(resolve=>uni.showModal({title:'修改尚未保存',content:error.value+'。离开会放弃本页未保存的修改，是否继续？',success:r=>resolve(!!r.confirm),fail:()=>resolve(false)}));if(discard&&alive){if(timer){clearTimeout(timer);timer=undefined;}hydrating=true;draft.value=saved.value?copy(saved.value):null;error.value='';conflict.value=false;hydrating=false;}return discard;}
 onBeforeUnmount(()=>{alive=false;if(timer)clearTimeout(timer);});
 return {draft,saved,loading,saving,error,conflict,dirty,status,load,flush,reload,prepareToLeave};
}
