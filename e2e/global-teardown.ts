/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import { ChildProcessWithoutNullStreams } from 'child_process';

declare const global: NodeJS.Global &
  typeof globalThis & { chromium: ChildProcessWithoutNullStreams };

/**
 * teardown kills the chromium instance when the test run is complete.
 */
export default async function teardown(): Promise<void> {
  if (global.chromium) {
    global.chromium.kill();
    await new Promise(r => global.chromium.on('exit', r));
  }
}
