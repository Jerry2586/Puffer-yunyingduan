import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs/promises';

test('embedded playback adapter stays identical to the legacy standalone installer source', async () => {
 const [embedded, standalone] = await Promise.all([
  fs.readFile(new URL('../../go-server/cms/Hetun.php', import.meta.url)),
  fs.readFile(new URL('../../server/maccms-extension/Hetun.php', import.meta.url)),
 ]);
 assert.deepEqual(embedded, standalone);
});
