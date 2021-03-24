import os from 'os';
import path from 'path';
import rimraf from 'rimraf';
import { Browser } from 'puppeteer';

declare const global: NodeJS.Global & typeof globalThis & { browser: Browser };

const DIR = path.join(os.tmpdir(), 'jest_puppeteer_global_setup');
export default async function teardown(): Promise<void> {
  await global.browser.close();
  // Clean-up the websocket endpoint file.
  rimraf.sync(DIR);
}
