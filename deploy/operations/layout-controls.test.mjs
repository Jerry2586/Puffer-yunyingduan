import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';
import {createRequire} from 'node:module';
import {fileURLToPath} from 'node:url';
const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'../..');
const local=createRequire(path.join(root,'package.json'));
const cached=createRequire(path.join(process.env.LOCALAPPDATA,'Programs/UniAppTools/projects/hetun-app-c7cb6e60b06b/runtime/workspace/package.json'));
let ts;try{ts=local('typescript');}catch{ts=cached('typescript');}
const mod={exports:{}};
new Function('module','exports',ts.transpileModule(fs.readFileSync(path.join(root,'src/domain/operation-layout.ts'),'utf8'),{compilerOptions:{module:ts.ModuleKind.CommonJS}}).outputText)(mod,mod.exports);
const {layoutNumbers,layoutValue,layoutColor,sectionLayout,menuLayout}=mod.exports;
const defaults=JSON.parse(fs.readFileSync(path.join(root,'src/domain/default-theme.json'),'utf8'));
test('invalid cached sizes and colors safely fall back without emitting arbitrary CSS',()=>{
 for(const [kind,rules] of Object.entries(layoutNumbers))for(const rule of rules){
  for(const value of [NaN,Infinity,rule.min-1,rule.max+1,rule.min+.5,'12',null])assert.equal(layoutValue(kind,{[rule.key]:value},rule.key),rule.default);
  assert.equal(layoutValue(kind,{[rule.key]:rule.min},rule.key),rule.min);
  assert.equal(layoutValue(kind,{[rule.key]:rule.max},rule.key),rule.max);
 }
 assert.equal(layoutColor({background:'url(https://example.test)'},'background','transparent'),'transparent');
 assert.equal(layoutColor({textColor:'#aAbB09'},'textColor','#ffffff'),'#aAbB09');
});
test('section dimensions are isolated and reset preserves existing section geometry and binding',()=>{
 const theme=structuredClone(defaults),a=theme.sections[0],b=structuredClone(a);
 const original=structuredClone(a);a.appearance={widthPercent:80,paddingX:0,cardWidth:160,headingColor:'#102030'};
 assert.equal(sectionLayout(theme,a).width,'80%');assert.equal(sectionLayout(theme,a)['--card-width'],'160px');
 assert.equal(sectionLayout(theme,b).width,'100%');assert.equal(sectionLayout(theme,b)['--card-width'],undefined);
 delete a.appearance;assert.deepEqual(a,original);assert.deepEqual(sectionLayout(theme,a),sectionLayout(theme,b));
});
test('menu style reset uses theme colors and does not change menu entries',()=>{
 const theme=structuredClone(defaults),original=structuredClone(theme.serviceMenuItems);
 theme.serviceMenuStyle={textColor:'#123456',rowHeight:80};assert.equal(menuLayout(theme)['--menu-row-height'],'80px');assert.equal(menuLayout(theme)['--menu-color'],'#123456');
 delete theme.serviceMenuStyle;assert.equal(menuLayout(theme)['--menu-color'],theme.colors.text);assert.deepEqual(theme.serviceMenuItems,original);
});
