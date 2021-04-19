/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { spawn, ChildProcessWithoutNullStreams } from 'child_process';
import wait from 'wait-port';

declare const global: NodeJS.Global &
  typeof globalThis & { chromium: ChildProcessWithoutNullStreams };

/**
 * port is the port the chrome instance will listen for connections on.
 * puppeteer will connect to ws://localhost:<port>, while the test debugger
 * is available at http://localhost:<port>. This must match the value in
 * ./test-environment.js.
 */
const port = Number(process.env.PORT) || 3000;

/**
 * setup starts a docker-ized instance of chrome, waits for the websocket port
 * that puppeteer will use to control chrome with to be listening for connections,
 * and sleeps momentarily to make sure everything is ready to go.
 */
export default async function setup(): Promise<void> {
  global.chromium = spawn(
    'docker',
    ['run', '--rm', '-p', `${port}:${port}`, 'browserless/chrome'],
    {
      stdio: 'ignore',
    }
  );

  global.chromium.on('error', e => {
    console.error(e);
    process.exit(1);
  });

  await wait({ port, output: 'dots' });
  await sleep(3000);
}

function sleep(ms: number) {
  return new Promise(resolve => {
    setTimeout(resolve, ms);
  });
}
