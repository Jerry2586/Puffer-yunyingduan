import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import {dependency} from './test-dependencies.mjs';
const ts=dependency('typescript'),mod={exports:{}};
new Function('module','exports',ts.transpileModule(fs.readFileSync(new URL('../../src/domain/installation-time.ts',import.meta.url),'utf8'),{compilerOptions:{module:ts.ModuleKind.CommonJS}}).outputText)(mod,mod.exports);
const {installationSeconds,installationDuration}=mod.exports;
test('installation age crosses midnight and keeps total days above 24 hours',()=>{
 const start='2026-10-01T00:00:00Z',checked='2026-11-05T23:59:59Z';
 assert.equal(installationDuration(installationSeconds(start,checked)), '35天 23:59:59');
 assert.equal(installationDuration(installationSeconds(start,checked,1000)), '36天 00:00:00');
});
test('reopened dashboard and service downtime retain installation age',()=>{
 const start='2026-10-01T00:00:00Z';
 assert.equal(installationSeconds(start,'2026-10-04T00:00:00Z'),3*86400);
 assert.equal(installationSeconds(start,'2026-11-06T00:00:00Z'),36*86400);
 assert.equal(installationSeconds(start,'2026-11-06T00:00:00Z',2500),36*86400+2);
});
test('missing installation timestamp is explicit and no fabricated fallback is used',()=>{
 assert.equal(installationSeconds(undefined,'2026-11-06T00:00:00Z'),null);
 assert.equal(installationDuration(null),'安装时间未记录');
 assert.equal(installationSeconds('bad','2026-11-06T00:00:00Z'),null);
 assert.equal(installationSeconds('2026-11-07T00:00:00Z','2026-11-06T00:00:00Z'),0);
});
