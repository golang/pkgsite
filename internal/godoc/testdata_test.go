// Copyright 2020 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package godoc

// quoteDocHTML is generated using the template in internal/fetch/dochtml from
// commit a6259ae.
const quoteDocHTML = quoteSidenav + quoteBody

const quoteSidenav = `<nav class="DocNav js-sideNav">
   <ul role="tree" aria-label="Outline">
      <li class="DocNav-overview" role="none">
         <a href="#pkg-overview" role="treeitem" aria-level="1" tabindex="0">Overview</a>
      </li>
      <li class="DocNav-functions" role="none">
         <span class="DocNav-groupLabel" role="treeitem" aria-expanded="true" aria-level="1" aria-owns="nav-group-functions" tabindex="-1">Functions</span>
         <ul role="group" id="nav-group-functions">
            <li role="none">
               <a href="#Glass" title="Glass()" role="treeitem" aria-level="2" tabindex="-1">Glass()</a>
            </li>
            <li role="none">
               <a href="#Go" title="Go()" role="treeitem" aria-level="2" tabindex="-1">Go()</a>
            </li>
            <li role="none">
               <a href="#Hello" title="Hello()" role="treeitem" aria-level="2" tabindex="-1">Hello()</a>
            </li>
            <li role="none">
               <a href="#Opt" title="Opt()" role="treeitem" aria-level="2" tabindex="-1">Opt()</a>
            </li>
         </ul>
      </li>
      <li class="DocNav-files" role="none">
         <a href="#pkg-files" role="treeitem" aria-level="1" tabindex="-1">Package Files</a>
      </li>
   </ul>
</nav>` + quoteSidenavMobile

const quoteSidenavMobile = `<nav class="DocNavMobile js-mobileNav">
   <label for="DocNavMobile-select" class="DocNavMobile-label">
      <svg class="DocNavMobile-selectIcon" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="black" width="18px" height="18px">
         <path d="M0 0h24v24H0z" fill="none"/>
         <path d="M3 9h14V7H3v2zm0 4h14v-2H3v2zm0 4h14v-2H3v2zm16 0h2v-2h-2v2zm0-10v2h2V7h-2zm0 6h2v-2h-2v2z"/>
      </svg>
      <span class="DocNavMobile-selectText js-mobileNavSelectText">Outline</span>
   </label>
   <select id="DocNavMobile-select" class="DocNavMobile-select">
      <option value="">Outline</option>
      <option value="pkg-overview">Overview</option>
      <optgroup label="Functions">
         <option value="Glass">Glass()</option>
         <option value="Go">Go()</option>
         <option value="Hello">Hello()</option>
         <option value="Opt">Opt()</option>
      </optgroup>
   </select>
</nav>`

const quoteBody = `<div class="Documentation-content js-docContent">
   <section class="Documentation-overview">
      <h3 tabindex="-1" id="pkg-overview" class="Documentation-overviewHeader">Overview <a href="#pkg-overview">¶</a></h3>
      <p>Package quote collects pithy sayings.</p>
   </section>
   <section class="Documentation-index">
      <h3 id="pkg-index" class="Documentation-indexHeader">Index <a href="#pkg-index">¶</a></h3>
      <ul class="Documentation-indexList">
         <li class="Documentation-indexFunction">
            <a href="#Glass">func Glass() string</a>
         </li>
         <li class="Documentation-indexFunction">
            <a href="#Go">func Go() string</a>
         </li>
         <li class="Documentation-indexFunction">
            <a href="#Hello">func Hello() string</a>
         </li>
         <li class="Documentation-indexFunction">
            <a href="#Opt">func Opt() string</a>
         </li>
      </ul>
   </section>
   <section class="Documentation-functions">
      <div class="Documentation-function">
         <h3 tabindex="-1" id="Glass" data-kind="function" class="Documentation-functionHeader">func <a class="Documentation-source" href="https://github.com/rsc/quote/blob/v1.5.2/quote.go#L16">Glass</a> <a href="#Glass">¶</a></h3>
         <pre>
func Glass() <a href="/builtin?tab=doc#string">string</a></pre>
         <p>Glass returns a useful phrase for world travelers.</p>
      </div>
      <div class="Documentation-function">
         <h3 tabindex="-1" id="Go" data-kind="function" class="Documentation-functionHeader">func <a class="Documentation-source" href="https://github.com/rsc/quote/blob/v1.5.2/quote.go#L22">Go</a> <a href="#Go">¶</a></h3>
         <pre>
func Go() <a href="/builtin?tab=doc#string">string</a></pre>
         <p>Go returns a Go proverb.</p>
      </div>
      <div class="Documentation-function">
         <h3 tabindex="-1" id="Hello" data-kind="function" class="Documentation-functionHeader">func <a class="Documentation-source" href="https://github.com/rsc/quote/blob/v1.5.2/quote.go#L11">Hello</a> <a href="#Hello">¶</a></h3>
         <pre>
func Hello() <a href="/builtin?tab=doc#string">string</a></pre>
         <p>Hello returns a greeting.</p>
      </div>
      <div class="Documentation-function">
         <h3 tabindex="-1" id="Opt" data-kind="function" class="Documentation-functionHeader">func <a class="Documentation-source" href="https://github.com/rsc/quote/blob/v1.5.2/quote.go#L27">Opt</a> <a href="#Opt">¶</a></h3>
         <pre>
func Opt() <a href="/builtin?tab=doc#string">string</a></pre>
         <p>Opt returns an optimization truth.</p>
      </div>
   </section>
</div>`
