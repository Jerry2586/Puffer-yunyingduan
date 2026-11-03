import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import {dependency} from './test-dependencies.mjs';
const root=path.resolve(import.meta.dirname,'../..');
const ts=dependency('typescript'),vue=dependency('vue'),{parse,compileScript}=dependency('@vue/compiler-sfc');
function picker(props){
 const file=path.join(root,'src/components/CmsCategoryBinding.vue');
 const {descriptor}=parse(fs.readFileSync(file,'utf8'),{filename:file});
 const script=compileScript(descriptor,{id:'binding-test'});
 const mod={exports:{}};
 new Function('require','module','exports',ts.transpileModule(script.content,{compilerOptions:{module:ts.ModuleKind.CommonJS}}).outputText)(()=>({...vue,onUnmounted:()=>{}}),mod,mod.exports);
 const events=[];const state=mod.exports.default.setup(props,{expose(){},emit:(...args)=>events.push(args)});
 return {state,events};
}
test('分类绑定从实际加载器更新一级分类，选择、改绑和解除发出真实 ID',async()=>{
 let calls=0;const props=vue.reactive({modelValue:'',categories:[],rootOnly:true,loadCategories:async()=>{calls++;return [{id:'1',name:'电影',parentId:'0'},{id:'11',name:'动作',parentId:'1'},{id:'2',name:'动漫',parentId:''}];}});
 const {state,events}=picker(props);await state.refresh();assert.equal(calls,1);assert.deepEqual(state.results.value.map(c=>c.id),['1','2']);
 state.choose('1');props.modelValue='1';assert.equal(state.current.value.name,'电影');
 state.choose('2');props.modelValue='2';assert.equal(state.current.value.name,'动漫');
 state.query.value='2';assert.deepEqual(state.results.value.map(c=>c.id),['2']);
 state.choose('');assert.deepEqual(events,[['update:modelValue','1'],['update:modelValue','2'],['update:modelValue','']]);
});
test('分类刷新失败可重试，过期请求不能覆盖较新分类',async()=>{
 let rejectFirst=true;const props={categories:[],rootOnly:true,loadCategories:async()=>{if(rejectFirst)throw Error('内容源不可用');return [{id:'new',name:'新分类'}];}};
 const {state}=picker(props);await state.refresh();assert.equal(state.error.value,'内容源不可用');assert.equal(state.loading.value,false);
 rejectFirst=false;await state.refresh();assert.equal(state.error.value,'');assert.equal(state.results.value[0].id,'new');
 const resolvers=[];props.loadCategories=()=>new Promise(resolve=>resolvers.push(resolve));
 const old=state.refresh(),fresh=state.refresh();resolvers[1]([{id:'latest',name:'最新'}]);await fresh;resolvers[0]([{id:'stale',name:'过期'}]);await old;
 assert.equal(state.results.value[0].id,'latest');
});
