/**
 * @license
 * Copyright 2026 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

const js = require('@eslint/js');
const tseslint = require('typescript-eslint');
const prettier = require('eslint-plugin-prettier/recommended');
const jest = require('eslint-plugin-jest');
const globals = require('globals');

module.exports = tseslint.config(
  {
    ignores: [
      'third_party/**',
      'content/static/js/**/*.js',
      'content/static/js/**/*.js.map',
      'static/**/*.js',
      'static/**/*.js.map',
      'node_modules/**',
    ],
  },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  prettier,
  {
    languageOptions: { globals: { ...globals.browser } },
  },
  {
    files: ['**/*.test.ts'],
    ...jest.configs['flat/recommended'],
    rules: { '@typescript-eslint/no-explicit-any': 'off' },
  }
);
