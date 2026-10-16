const assert = require('node:assert/strict');
exports.saveRecovery = async function(page) {
  await page.locator('#recovery-download').waitFor();
  assert(await page.locator('#recovery-finish').isDisabled());
  assert(await page.locator('#recovery-saved').isDisabled());
  const downloaded=page.waitForEvent('download');
  await page.locator('#recovery-download').click();
  const download=await downloaded;
  if(await download.failure()) throw Error('Recovery TXT download failed');
  const fs=require('node:fs');
  const text=fs.readFileSync(await download.path(),'utf8');
  const key=text.match(/UFIG(?:-[A-F0-9]{8}){8}/)?.[0];
  assert(key,'TXT must contain a high-entropy key');
  await page.locator('#recovery-saved').check();
  await page.locator('#recovery-finish').click();
  return key;
};
