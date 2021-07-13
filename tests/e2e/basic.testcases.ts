/**
 * @license
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

interface TestArgs {
  name: string;
  path: string;
}

interface TestCases {
  (name: string, fn: (arg: TestArgs) => unknown, timeout?: number): unknown;
}

export const testcases: TestCases = test.each`
  name                       | path
  ${'badge'}                 | ${'/badge'}
  ${'error'}                 | ${'/bad.package@v1.0-badversion'}
  ${'404 with fetch button'} | ${'/github.com/package/does/not/exist'}
  ${'home'}                  | ${'/'}
  ${'license policy'}        | ${'/license-policy'}
  ${'search help'}           | ${'/search-help'}
`;
