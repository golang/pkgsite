/**
 * @license
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

import './global-types';
import * as pg from './helpers/page';

interface TestArgs {
  name: string;
  path: string;
}

interface TestCases {
  (name: string, fn: (arg: TestArgs) => unknown, timeout?: number): unknown;
}

const testcases: TestCases = test.each`
  name                       | path
  ${'badge'}                 | ${'/badge'}
  ${'error'}                 | ${'/bad.package@v1.0-badversion'}
  ${'404 with fetch button'} | ${'/github.com/package/does/not/exist'}
  ${'home'}                  | ${'/'}
  ${'license policy'}        | ${'/license-policy'}
  ${'search help'}           | ${'/search-help'}
`;

testcases('desktop $name accessibility tree', async args => {
  const page = await pg.newPage();
  await pg.a11ySnapshotTest(page, args);
});

testcases('desktop $name screenshot', async args => {
  const page = await pg.newPage();
  await pg.fullScreenshotTest(page, args);
});

testcases('mobile $name accessibility tree', async args => {
  const page = await pg.newPage();
  await pg.a11ySnapshotTest(page, { ...args, mobile: true });
});

testcases('mobile $name screenshot', async args => {
  const page = await pg.newPage();
  await pg.fullScreenshotTest(page, { ...args, mobile: true });
});

test('no page errors', () => {
  expect(pageErrors).toHaveLength(0);
});
