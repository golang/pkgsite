/**
 * @license
 * Copyright 2024 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */

/**
 * 站点 URL 子路径前缀，从 <html data-base-path="..."> 读出。
 *
 * fork 用 -base-path=/gogodocs 启动时，server 端模板把 attribute 写成
 * "/gogodocs"；上游公网 pkg.go.dev 默认挂根，attribute 为空字符串。
 *
 * 在 <head> 之外的脚本初始化阶段调用都安全——documentElement 一定存在。
 */
export function getBasePath(): string {
  return document.documentElement.dataset.basePath ?? '';
}

/**
 * 给以 / 开头的绝对路径加 BasePath 前缀。
 *
 * 用于 fetch URL / innerHTML src / location 比较等任何需要拼站点绝对 path
 * 的地方。非绝对路径（不以 / 开头）原样返回——caller 自己保证语义。
 *
 * 例：
 *   abs('/play/share')       // → '/gogodocs/play/share' 或 '/play/share'
 *   abs('/static/foo.svg')   // → '/gogodocs/static/foo.svg' 或 '/static/foo.svg'
 *   abs('relative/x')        // → 'relative/x'（不动）
 */
export function abs(p: string): string {
  if (!p.startsWith('/')) return p;
  return getBasePath() + p;
}
