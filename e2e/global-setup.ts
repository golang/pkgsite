import fs from 'fs';
import os from 'os';
import path from 'path';
import mkdirp from 'mkdirp';
import puppeteer, { Browser } from 'puppeteer';

declare const global: NodeJS.Global & typeof globalThis & { browser: Browser };

const DIR = path.join(os.tmpdir(), 'jest_puppeteer_global_setup');
export default async function setup(): Promise<void> {
  global.browser = await puppeteer.launch({
    args: ['--no-sandbox', '--disable-dev-shm-usage'],
  });

  // Writing the websocket endpoint to a file so that tests
  // can use it to connect to a global browser instance.
  mkdirp.sync(DIR);
  fs.writeFileSync(path.join(DIR, 'wsEndpoint'), global.browser.wsEndpoint());
}
