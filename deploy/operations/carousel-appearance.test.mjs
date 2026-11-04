import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import {dependency} from './test-dependencies.mjs';
const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'../..');
const ts=dependency('typescript'),vue=dependency('vue');
const defaults=JSON.parse(fs.readFileSync(path.join(root,'src/domain/default-theme.json'),'utf8'));
const compile=source=>ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.CommonJS}}).outputText;
const domainModule={exports:{}};
new Function('module','exports',compile(fs.readFileSync(path.join(root,'src/domain/carousel-appearance.ts'),'utf8')))(domainModule,domainModule.exports);
const rules=domainModule.exports;
function editor(initial){
 const props=vue.reactive({base:structuredClone(defaults.carousel),modelValue:initial}),events=[];
 const source=fs.readFileSync(path.join(root,'src/components/OperationsCarouselAppearance.vue'),'utf8').match(/<script setup lang="ts">([\s\S]*?)<\/script>/)[1];
 const mod={exports:{}};
 new Function('require','module','exports','defineProps','defineEmits',compile(source+'\nexport {number,set,reset,geometry,drafts};'))(name=>name==='vue'?vue:name==='@/domain/carousel-appearance'?rules:{},mod,mod.exports,()=>props,()=>((event,value)=>{events.push([event,value]);props.modelValue=value;}));
 return {props,events,...mod.exports};
}
test('legacy theme fallback and invalid cache never emit unsafe geometry or mutate sources',()=>{
 const base=structuredClone(defaults.carousel),original=structuredClone(base);
 const fallback=rules.resolveCarouselAppearance(base);
 assert.deepEqual(fallback,{style:base.style,height:base.height,radius:base.radius,margin:base.margin,fit:base.fit,widthPercent:100});
 for(const field of rules.carouselNumbers){
  for(const value of [NaN,Infinity,field.min-1,field.max+1,field.min+.5,'240',null]){
   assert.equal(rules.resolveCarouselAppearance(base,{[field.key]:value})[field.key],fallback[field.key]);
  }
 }
 assert.deepEqual(rules.resolveCarouselAppearance(base,{style:'url(bad)',fit:'fill'}),fallback);
 assert.deepEqual(base,original);
});
test('page overrides render dimensions independently and resetting retains theme and content',()=>{
 const base=structuredClone(defaults.carousel),before=structuredClone(base);
 const a={style:'split',widthPercent:80,height:360,radius:180,margin:16,fit:'contain'};
 const b={style:'poster',height:480,radius:0};
 const frame=rules.carouselFrameStyle(rules.resolveCarouselAppearance(base,a));
 assert.equal(frame.width,'calc(80% - 32px)');assert.equal(frame['--height'],'360px');assert.equal(frame['--radius'],'180px');assert.equal(frame['--fit'],'contain');
 assert.equal(rules.resolveCarouselAppearance(base,b).height,480);
 assert.equal(rules.resolveCarouselAppearance(base,b).widthPercent,100);
 assert.deepEqual(rules.resolveCarouselAppearance(base),rules.resolveCarouselAppearance(before));
 assert.deepEqual(base,before);assert.equal(a.height,360);assert.equal(b.height,480);
});
test('number edits autosave valid values, clamp at blur, rehydrate, and reset all overrides',async()=>{
 const e=editor({height:600,radius:12});
 e.number('height',{detail:{value:'800'}});assert.equal(e.props.modelValue.height,600);assert.equal(e.drafts.height,'800');
 e.number('height',{detail:{value:'800'}},true);assert.equal(e.geometry.value.height,600);assert.equal(e.drafts.height,undefined);
 e.number('height',{detail:{value:'320'}});assert.equal(e.props.modelValue.height,320);
 e.props.modelValue=JSON.parse(JSON.stringify(e.props.modelValue));await vue.nextTick();assert.equal(e.geometry.value.height,320);
 e.set('fit','contain');e.set('style','split');assert.equal(e.geometry.value.fit,'contain');assert.equal(e.geometry.value.style,'split');
 e.number('radius',{detail:{value:'-1'}},true);assert.equal(e.geometry.value.radius,0);
 e.number('height',{detail:{value:''}});assert.equal(e.geometry.value.height,e.props.base.height);
 e.reset();assert.equal(e.props.modelValue,undefined);assert.deepEqual(e.drafts,{});
 assert.deepEqual(e.events.at(-1),['update:modelValue',undefined]);
});
test('renderer consumes page geometry, image fit and side-peek with either theme effect',()=>{
 const props=vue.reactive({theme:structuredClone(defaults),items:[],editing:true,operationsSlides:[],appearance:{style:'peek',height:300,fit:'contain'}}),scope=vue.effectScope(),dispose=[];
 props.theme.carousel.effect='fade';
 const source=fs.readFileSync(path.join(root,'src/components/ThemeCarousel.vue'),'utf8').match(/<script setup lang="ts">([\s\S]*?)<\/script>/)[1],mod={exports:{}};
 scope.run(()=>new Function('require','module','exports','defineProps','defineEmits',compile(source+'\nexport {geometry,frameStyle,usesFade};'))(name=>name==='vue'?{...vue,onUnmounted:fn=>dispose.push(fn)}:name==='@/domain/carousel-appearance'?rules:name==='@/services/api'?{assetUrl:x=>x,api:{}}:{},mod,mod.exports,()=>props,()=>()=>{}));
 try{
  assert.equal(mod.exports.usesFade.value,false);assert.equal(mod.exports.frameStyle.value['--height'],'300px');assert.equal(mod.exports.geometry.value.fit,'contain');
  props.appearance={style:'split',radius:90};assert.equal(mod.exports.usesFade.value,true);assert.equal(mod.exports.frameStyle.value['--radius'],'90px');
  props.appearance=undefined;assert.equal(mod.exports.geometry.value.height,defaults.carousel.height);
 }finally{dispose.forEach(fn=>fn());scope.stop();}
});
