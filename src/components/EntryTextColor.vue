<script lang="ts">
import {defineComponent,h} from 'vue';
// Render a native input so uni-app does not turn the browser color picker into a text field.
export default defineComponent({
 name:'EntryTextColor',
 props:{label:{type:String,required:true},modelValue:String,defaultColor:String},
 emits:{'update:modelValue':(value:string|undefined)=>value===undefined||/^#[0-9a-fA-F]{6}$/.test(value)},
 setup(props,{emit}){
  function update(event:Event){const value=(event.target as HTMLInputElement).value;if(/^#[0-9a-fA-F]{6}$/.test(value))emit('update:modelValue',value);}
  return ()=>h('div',{class:'entry-text-color'},[
   h('label',{class:'color-wheel',title:props.label+'字体颜色'},[
    h('input',{class:'color-input',type:'color',value:props.modelValue||(/^#[0-9a-fA-F]{6}$/.test(props.defaultColor||'')?props.defaultColor:'#ffffff'),'aria-label':props.label+'字体颜色',onInput:update,onChange:update})
   ]),
   props.modelValue?h('button',{type:'button',class:'reset-color',title:'恢复主题默认颜色','aria-label':props.label+'恢复默认字体颜色',onClick:()=>emit('update:modelValue',undefined)},'↺'):null
  ]);
 }
});
</script>
<style scoped>
.entry-text-color{display:flex;align-items:center;gap:4px;flex:none}.color-wheel{position:relative;display:block;width:28px;height:28px;flex:none;border-radius:50%;background:conic-gradient(#ff476a,#ffcc48,#53db86,#45c8ff,#9365ed,#ff476a);box-shadow:inset 0 0 0 1px rgba(255,255,255,.2);cursor:pointer}.color-wheel::after{content:'';position:absolute;inset:8px;border-radius:50%;background:var(--studio-surface,#152027);pointer-events:none}.color-wheel:focus-within{outline:2px solid var(--studio-text,#e7edf3);outline-offset:3px}.color-input{position:absolute;inset:0;width:100%;height:100%;opacity:0;cursor:pointer;border:0;padding:0}.reset-color{box-sizing:border-box;flex:none;margin:0;line-height:1;display:flex;align-items:center;justify-content:center;width:24px;height:28px;border:0;border-radius:4px;padding:0;background:transparent;color:var(--studio-muted,#91a9a0);font-size:18px;cursor:pointer}.reset-color:hover,.reset-color:focus-visible{color:var(--studio-text,#e7edf3);background:var(--studio-input,#172329)}
</style>
