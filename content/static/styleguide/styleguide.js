/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */import{SelectNavController as i,makeSelectNav as n}from"../_outline/select.js";import{TreeNavController as o}from"../_outline/tree.js";window.addEventListener("load",()=>{const e=document.querySelector(".js-tree");if(e){const t=new o(e),r=n(t);document.querySelector(".js-mainNavMobile").appendChild(r)}const s=document.querySelector(".Outline .js-tree");if(s){const t=new o(s),r=n(t);document.querySelector(".Outline .js-select").appendChild(r)}for(const t of document.querySelectorAll(".js-toggleTheme"))t.addEventListener("click",r=>{const l=r.currentTarget.getAttribute("data-value");document.documentElement.setAttribute("data-theme",l)});for(const t of document.querySelectorAll(".js-toggleLayout"))t.addEventListener("click",r=>{const l=r.currentTarget.getAttribute("data-value");document.documentElement.setAttribute("data-layout",l)});for(const t of document.querySelectorAll(".js-selectNav"))new i(t)}),customElements.define("go-color",class extends HTMLElement{constructor(){super();this.style.setProperty("display","contents");const e=this.id;this.removeAttribute("id"),this.innerHTML=`
        <div style="--color: var(${e});" class="GoColor-circle"></div>
        <span>
          <div id="${e}" class="go-textLabel GoColor-title">${e.replace("--color-","").replaceAll("-"," ")}</div>
          <pre class="StringifyElement-markup">var(${e})</pre>
        </span>
      `,this.querySelector("pre").onclick=()=>{navigator.clipboard.writeText(`var(${e})`)}}}),customElements.define("go-icon",class extends HTMLElement{constructor(){super();this.style.setProperty("display","contents");const e=this.getAttribute("name");this.innerHTML=`
        <p id="icon-${e}" class="go-textLabel GoIcon-title">${e.replaceAll("_"," ")}</p>
        <stringify-el>
          <img class="go-Icon" height="24" width="24" src="/static/_icon/${e}_gm_grey_24dp.svg" alt="">
        </stringify-el>
      `}}),customElements.define("clone-el",class extends HTMLElement{constructor(){super();this.style.setProperty("display","contents");const e=this.getAttribute("selector"),s="    "+document.querySelector(e).outerHTML;this.innerHTML=`
        <stringify-el collapsed>${s}</stringify-el>
      `}}),customElements.define("stringify-el",class extends HTMLElement{constructor(){super();this.style.setProperty("display","contents");const e=this.innerHTML,s=this.id?` id="${this.id}"`:"";this.removeAttribute("id");let t='<pre class="StringifyElement-markup">'+a(c(e))+"</pre>";this.hasAttribute("collapsed")&&(t=`<details class="StringifyElement-details"><summary>Markup</summary>${t}</details>`),this.innerHTML=`<span${s}>${e}</span>${t}`,this.querySelector("pre").onclick=()=>{navigator.clipboard.writeText(e)}}});function c(e){return e.split(`
`).reduce((s,t)=>{if(s.result.length===0){const r=t.indexOf("<");s.start=r===-1?0:r}return t=t.slice(s.start),t&&s.result.push(t),s},{result:[],start:0}).result.join(`
`)}function a(e){return e.replaceAll("<","&lt;").replaceAll(">","&gt;")}
//# sourceMappingURL=styleguide.js.map
