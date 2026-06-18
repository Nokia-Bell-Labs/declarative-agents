(function(){const t=document.createElement("link").relList;if(t&&t.supports&&t.supports("modulepreload"))return;for(const e of document.querySelectorAll('link[rel="modulepreload"]'))o(e);new MutationObserver(e=>{for(const s of e)if(s.type==="childList")for(const i of s.addedNodes)i.tagName==="LINK"&&i.rel==="modulepreload"&&o(i)}).observe(document,{childList:!0,subtree:!0});function r(e){const s={};return e.integrity&&(s.integrity=e.integrity),e.referrerPolicy&&(s.referrerPolicy=e.referrerPolicy),e.crossOrigin==="use-credentials"?s.credentials="include":e.crossOrigin==="anonymous"?s.credentials="omit":s.credentials="same-origin",s}function o(e){if(e.ep)return;e.ep=!0;const s=r(e);fetch(e.href,s)}})();const d={state:"/monitor/state",machine:"/monitor/machine",tools:"/monitor/tools",events:"/monitor/events",stream:"/monitor/events/stream"};function a(n){const t=document.getElementById(n);if(!t)throw new Error(`missing #${n}`);return t}async function l(n){const t=await fetch(n);if(!t.ok)throw new Error(`${n} -> HTTP ${t.status}`);return t.json()}function h(n,t){a(n).textContent=t}function m(n,t){a(n).textContent=JSON.stringify(t,null,2)}function p(n){const t=n.run;if(!t){h("run-summary","No run snapshot yet.");return}const r=["run_id","status","state","signal","iteration"].map(o=>`${o}=${String(t[o]??"")}`);h("run-summary",r.join(" | "))}function v(n){const t=a("transition-rows");t.replaceChildren();const r=n.transitions;if(!(r!=null&&r.length)){const o=document.createElement("tr"),e=document.createElement("td");e.colSpan=5,e.textContent="No transitions in machine snapshot.",o.appendChild(e),t.appendChild(o);return}for(const o of r){const e=document.createElement("tr");for(const s of["state","signal","next","action","metric_labels"]){const i=document.createElement("td"),c=o[s];i.textContent=typeof c=="object"&&c!==null?JSON.stringify(c):String(c??""),e.appendChild(i)}t.appendChild(e)}}function g(n){const t=a("tool-rows");t.replaceChildren();const r=n.tools;if(!(r!=null&&r.length)){const o=document.createElement("tr"),e=document.createElement("td");e.colSpan=4,e.textContent="No tools registered.",o.appendChild(e),t.appendChild(o);return}for(const o of r){const e=document.createElement("tr");for(const s of["name","category","visibility","emits"]){const i=document.createElement("td"),c=o[s];i.textContent=Array.isArray(c)?c.join(", "):String(c??""),e.appendChild(i)}t.appendChild(e)}}function y(n){const t=n.recent_events;m("recent-events",t??[])}function u(n){var o;const t=a("stream-feed"),r=document.createElement("div");for(r.className="feed-row",r.textContent=n,t.prepend(r);t.childNodes.length>120;)(o=t.lastChild)==null||o.remove()}function w(){const n=new EventSource(d.stream),t=r=>o=>{u(`${new Date().toISOString()} [${r}] ${o.data}`)};n.addEventListener("run_event",t("run_event")),n.addEventListener("metric_sample",t("metric_sample")),n.onerror=()=>{u(`${new Date().toISOString()} [eventsource] connection error (retrying)`)}}function S(){return`
    <h1>Knowledge Manager Monitor</h1>
    <div id="status" class="banner">Loading monitor snapshot…</div>
    <div class="grid">
      <section class="panel">
        <h2>Run</h2>
        <div id="run-summary" class="feed"></div>
      </section>
      <section class="panel">
        <h2>State (JSON)</h2>
        <pre id="state-json"></pre>
      </section>
      <section class="panel full-width">
        <h2>Machine transitions</h2>
        <table>
          <thead><tr><th>state</th><th>signal</th><th>next</th><th>action</th><th>metric_labels</th></tr></thead>
          <tbody id="transition-rows"></tbody>
        </table>
        <h2 style="margin-top:0.75rem">Machine (JSON)</h2>
        <pre id="machine-json"></pre>
      </section>
      <section class="panel">
        <h2>Tools</h2>
        <table>
          <thead><tr><th>name</th><th>category</th><th>visibility</th><th>emits</th></tr></thead>
          <tbody id="tool-rows"></tbody>
        </table>
      </section>
      <section class="panel">
        <h2>Recent events (JSON)</h2>
        <pre id="recent-events"></pre>
      </section>
      <section class="panel full-width">
        <h2>Live stream (/monitor/events/stream)</h2>
        <div id="stream-feed" class="feed"></div>
      </section>
    </div>
  `}async function f(){try{const[n,t,r,o]=await Promise.all([l(d.state),l(d.machine),l(d.tools),l(d.events)]),e=n,s=t,i=r,c=o;h("status","Connected to monitor API."),p(e),m("state-json",e),m("machine-json",s),v(s),g(i),y(c)}catch(n){const t=n instanceof Error?n.message:String(n);h("status",`Monitor API error: ${t}`)}}function b(){a("app").innerHTML=S(),w(),f(),window.setInterval(()=>void f(),4e3)}b();
