(function(){
  var REPO = "wagneripjr/kubectl-tessera";
  var OSLABEL = {linux:"Linux", darwin:"macOS", windows:"Windows"};
  var ARCHLABEL = {amd64:"x86‑64 (amd64)", arm64:"ARM64 (arm64)"};
  var ORDER = ["linux/amd64","linux/arm64","darwin/arm64","darwin/amd64","windows/amd64","windows/arm64"];

  // copy buttons
  document.addEventListener("click", function(e){
    var b = e.target.closest(".copy"); if(!b) return;
    var txt = b.getAttribute("data-copy") || "";
    navigator.clipboard.writeText(txt).then(function(){
      var old = b.textContent; b.textContent = "copied ✓"; b.classList.add("done");
      setTimeout(function(){ b.textContent = old; b.classList.remove("done"); }, 1600);
    });
  });

  // detect host platform to recommend a build
  function hostKey(){
    var p = ((navigator.userAgentData && navigator.userAgentData.platform) || navigator.platform || "").toLowerCase();
    var ua = navigator.userAgent.toLowerCase();
    var os = p.indexOf("win")>-1 ? "windows" : (p.indexOf("mac")>-1 ? "darwin" : "linux");
    var arch = (/arm|aarch64/.test(ua) || /arm/.test(p)) ? "arm64" : "amd64";
    return os + "/" + arch;
  }

  function showFallback(){ document.getElementById("dlfallback").classList.add("show"); }

  function render(assets, host){
    var grid = document.getElementById("dlgrid");
    var byKey = {};
    assets.forEach(function(a){
      var m = a.name.match(/_(linux|darwin|windows)_(amd64|arm64)\.(tar\.gz|zip)$/);
      // Only trust https download URLs (defends against a javascript:/data: asset URL).
      if(m && /^https:\/\//.test(a.browser_download_url||"")) byKey[m[1]+"/"+m[2]] = a.browser_download_url;
    });
    var keys = ORDER.filter(function(k){ return byKey[k]; });
    if(!keys.length){ showFallback(); return; }
    grid.textContent = "";
    keys.forEach(function(k){
      var os = k.split("/")[0], arch = k.split("/")[1];
      var a = document.createElement("a");
      a.className = "dl" + (k===host ? " rec" : "");
      a.href = byKey[k]; a.rel = "noopener";
      // Build with textContent only — no innerHTML, no untrusted interpolation.
      var osSpan = document.createElement("span");
      osSpan.className = "os";
      osSpan.appendChild(document.createTextNode(OSLABEL[os] || os));
      var small = document.createElement("small");
      small.textContent = ARCHLABEL[arch] || arch;
      osSpan.appendChild(small);
      var get = document.createElement("span");
      get.className = "get";
      get.textContent = "download ↓";
      a.appendChild(osSpan);
      a.appendChild(get);
      grid.appendChild(a);
    });
  }

  fetch("https://api.github.com/repos/"+REPO+"/releases/latest", {headers:{Accept:"application/vnd.github+json"}})
    .then(function(r){ if(!r.ok) throw new Error("no release ("+r.status+")"); return r.json(); })
    .then(function(rel){
      var tag = rel.tag_name || "latest";
      document.getElementById("vertag").textContent = tag;
      var assets = (rel.assets||[]).filter(function(a){ return /\.(tar\.gz|zip)$/.test(a.name); });
      if(!assets.length){ document.getElementById("vertag").textContent = tag+" (no binaries)"; showFallback(); return; }
      render(assets, hostKey());
    })
    .catch(function(){
      document.getElementById("vertag").textContent = "none yet";
      showFallback();
    });
})();
