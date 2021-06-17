/**
 * @license
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import './global-types';
import * as pg from './helpers/page';
import { testcases } from './basic.testcases';

testcases('$name accessibility tree', async args => {
  const page = await pg.newPage();
  await pg.a11ySnapshotTest(page, args);
});

testcases('$name screenshot', async args => {
  const page = await pg.newPage();
  await pg.fullScreenshotTest(page, args);
});
test('no page errors', () => {
  expect(pageErrors).toHaveLength(0);
});
