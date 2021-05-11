/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { spawn, ChildProcessWithoutNullStreams } from 'child_process';
import net from 'net';
import wait from 'wait-port';

declare const global: NodeJS.Global &
  typeof globalThis & { chromium: ChildProcessWithoutNullStreams };

/**
 * port is the port the chrome instance will listen for connections on.
 * puppeteer will connect to ws://localhost:<port>, while the test debugger
 * is available at http://localhost:<port>. This must match the value in
 * ./test-environment.js.
 */
const port = Number(process.env.GO_DISCOVERY_E2E_TEST_PORT) || 3000;

/**
 * setup starts a headless instance of chrome if necessary, waits for the websocket
 * port that puppeteer will use to control chrome with to be listening for connections,
 * and sleeps momentarily to make sure everything is ready to go.
 */
export default async function setup(): Promise<void> {
  const startServer = await portAvailable(port);
  if (startServer) {
    console.log(`\nStarting headless chrome on port ${port}...`);
    global.chromium = spawn('docker', ['run', '--rm', '-p', `${port}:3000`, 'browserless/chrome'], {
      stdio: 'ignore',
    });
    global.chromium.on('error', e => {
      console.error(e);
      process.exit(1);
    });
  }

  console.log('');
  await wait({ port, output: 'dots' });
  await sleep(3000);
}

function sleep(ms: number) {
  return new Promise(resolve => {
    setTimeout(resolve, ms);
  });
}

/**
 * portAvailable determines if a port is available for use by creating a temporary
 * server and testing the connection.
 * @param port the port to test
 * @returns true if the port is availabe.
 */
function portAvailable(port) {
  return new Promise<boolean>((resolve, reject) => {
    const tester = net
      .createServer()
      .once('error', err => (err.code == 'EADDRINUSE' ? resolve(false) : reject(err)))
      .once('listening', () => tester.once('close', () => resolve(true)).close())
      .listen(port);
  });
}
