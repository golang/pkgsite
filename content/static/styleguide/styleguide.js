/**
 * @license
 * Copyright 2021 The Go Authors. All rights reserved.
 * Use of this source code is governed by a BSD-style
 * license that can be found in the LICENSE file.
 */import{ClipboardController as a}from"../clipboard/clipboard.js";import{SelectNavController as u,makeSelectNav as i}from"../outline/select.js";import{ToolTipController as m}from"../tooltip/tooltip.js";import{TreeNavController as c}from"../outline/tree.js";import{MainLayoutController as d}from"../main-layout/main-layout.js";import{ModalController as p}from"../modal/modal.js";window.addEventListener("load",()=>{const e=document.querySelector(".js-tree");if(e){const t=new c(e),l=i(t);document.querySelector(".js-mainNavMobile").appendChild(l)}const o=document.querySelector(".Outline .js-tree");if(o){const t=new c(o),l=i(t);document.querySelector(".Outline .js-select").appendChild(l)}for(const t of document.querySelectorAll(".js-toggleTheme"))t.addEventListener("click",l=>{const s=l.currentTarget.getAttribute("data-value");document.documentElement.setAttribute("data-theme",s)});for(const t of document.querySelectorAll(".js-toggleLayout"))t.addEventListener("click",l=>{const s=l.currentTarget.getAttribute("data-value");document.documentElement.setAttribute("data-layout",s)});for(const t of document.querySelectorAll(".js-clipboard"))new a(t);for(const t of document.querySelectorAll(".js-selectNav"))new u(t);for(const t of document.querySelectorAll(".js-tooltip"))new m(t);for(const t of document.querySelectorAll(".js-modal"))new p(t);const r=document.querySelector(".js-mainHeader"),n=document.querySelector(".js-mainNav");new d(r,n)}),customElements.define("go-color",class extends HTMLElement{constructor(){super();this.style.setProperty("display","contents");const e=this.id;this.removeAttribute("id"),this.innerHTML=`
        <div style="--color: var(${e});" class="GoColor-circle"></div>
        <span>
          <div id="${e}" class="go-textLabel GoColor-title">${e.replace("--color-","").replaceAll("-"," ")}</div>
          <pre class="StringifyElement-markup">var(${e})</pre>
        </span>
      `,this.querySelector("pre").onclick=()=>{navigator.clipboard.writeText(`var(${e})`)}}}),customElements.define("go-icon",class extends HTMLElement{constructor(){super();this.style.setProperty("display","contents");const e=this.getAttribute("name");this.innerHTML=`
        <p id="icon-${e}" class="go-textLabel GoIcon-title">${e.replaceAll("_"," ")}</p>
        <stringify-el>
          <img class="go-Icon" height="24" width="24" src="/static/icon/${e}_gm_grey_24dp.svg" alt="">
        </stringify-el>
      `}}),customElements.define("clone-el",class extends HTMLElement{constructor(){super();this.style.setProperty("display","contents");const e=this.getAttribute("selector"),o="    "+document.querySelector(e).outerHTML;this.innerHTML=`
        <stringify-el collapsed>${o}</stringify-el>
      `}}),customElements.define("stringify-el",class extends HTMLElement{constructor(){super();this.style.setProperty("display","contents");const e=this.innerHTML,o=this.id?` id="${this.id}"`:"";this.removeAttribute("id");let r='<pre class="StringifyElement-markup">'+f(y(e))+"</pre>";this.hasAttribute("collapsed")&&(r=`<details class="StringifyElement-details"><summary>Markup</summary>${r}</details>`),this.innerHTML=`<span${o}>${e}</span>${r}`,this.querySelector("pre").onclick=()=>{navigator.clipboard.writeText(e)}}});function y(e){return e.split(`
`).reduce((o,r)=>{if(o.result.length===0){const n=r.indexOf("<");o.start=n===-1?0:n}return r=r.slice(o.start),r&&o.result.push(r),o},{result:[],start:0}).result.join(`
`)}function f(e){return e.replaceAll("<","&lt;").replaceAll(">","&gt;")}
//# sourceMappingURL=styleguide.js.map
