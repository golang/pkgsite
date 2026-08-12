/*!
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */
var n=document.querySelector(".js-siteHeader"),d=document.createElement("div");n==null||n.prepend(d);var u=new IntersectionObserver(([r])=>{if(r.intersectionRatio<1)for(let e of document.querySelectorAll('[class^="SearchResults-header"'))e.setAttribute("data-fixed","true");else for(let e of document.querySelectorAll('[class^="SearchResults-header"'))e.removeAttribute("data-fixed")},{threshold:1,rootMargin:`${3.5*16*3}px`});u.observe(d);var o=document.querySelector(".js-searchHeader");o==null||o.addEventListener("dblclick",r=>{var t;let e=r.target;(e===o||e===o.lastElementChild)&&((t=window.getSelection())==null||t.removeAllRanges(),window.scrollTo({top:0,behavior:"smooth"}))});document.addEventListener("click",r=>{let e=r.target,t=e==null?void 0:e.closest("a[data-search-query]");if(!t)return;let a=t.getAttribute("data-search-query"),c=t.getAttribute("data-clicked-package"),s=t.getAttribute("data-rank"),i=t.getAttribute("data-experiment-cohort");if(a&&c&&s&&i){let l={query:a,clicked_package:c,rank:parseInt(s,10),cohort:i,timestamp:new Date().toISOString()};navigator.sendBeacon&&navigator.sendBeacon("/search-click",JSON.stringify(l))}});
/**
 * @license
 * Copyright 2020 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */
//# sourceMappingURL=search.js.map
