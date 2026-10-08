// Envisible landing — OS detection, platform tabs, copy-to-clipboard.
(function () {
  "use strict";

  // Install command per platform. macOS/Linux use the shell installer;
  // native Windows uses the PowerShell installer. Both download the matching
  // prebuilt binary from the latest GitHub release.
  var COMMANDS = {
    mac: "curl -fsSL https://raw.githubusercontent.com/umairx25/Envisible/main/scripts/install.sh | sh",
    linux: "curl -fsSL https://raw.githubusercontent.com/umairx25/Envisible/main/scripts/install.sh | sh",
    windows: "irm https://raw.githubusercontent.com/umairx25/Envisible/main/scripts/install.ps1 | iex",
  };

  var tabs = document.querySelectorAll(".install-tab");
  var cmdEl = document.getElementById("install-command");
  var copyBtn = document.getElementById("copy-btn");

  function selectOS(os) {
    if (!COMMANDS[os]) os = "mac";
    cmdEl.textContent = COMMANDS[os];
    tabs.forEach(function (t) {
      var active = t.getAttribute("data-os") === os;
      t.classList.toggle("is-active", active);
      t.setAttribute("aria-selected", active ? "true" : "false");
    });
  }

  // Best-effort OS detection for the default tab.
  function detectOS() {
    var ua = (navigator.userAgent || "").toLowerCase();
    var platform = (navigator.platform || "").toLowerCase();
    if (platform.indexOf("mac") !== -1 || ua.indexOf("mac") !== -1) return "mac";
    if (ua.indexOf("win") !== -1 || platform.indexOf("win") !== -1) return "windows";
    if (ua.indexOf("linux") !== -1 || ua.indexOf("x11") !== -1) return "linux";
    return "mac";
  }

  tabs.forEach(function (t) {
    t.addEventListener("click", function () {
      selectOS(t.getAttribute("data-os"));
    });
  });

  // Generic copy helper: copies `text`, flashes the button to "Copied".
  function wireCopy(btn, getText) {
    if (!btn) return;
    btn.addEventListener("click", function () {
      var text = getText();
      var done = function () {
        var original = btn.getAttribute("data-label") || "Copy";
        btn.textContent = "Copied";
        btn.classList.add("copied");
        setTimeout(function () {
          btn.textContent = original;
          btn.classList.remove("copied");
        }, 1800);
      };
      if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(text).then(done).catch(fallbackCopy);
      } else {
        fallbackCopy();
      }
      function fallbackCopy() {
        var ta = document.createElement("textarea");
        ta.value = text;
        ta.style.position = "fixed";
        ta.style.opacity = "0";
        document.body.appendChild(ta);
        ta.select();
        try { document.execCommand("copy"); done(); } catch (e) { /* no-op */ }
        document.body.removeChild(ta);
      }
    });
  }

  wireCopy(copyBtn, function () { return cmdEl.textContent; });

  var promptCopy = document.getElementById("prompt-copy");
  var promptEl = document.getElementById("agent-prompt");
  if (promptCopy && promptEl) {
    wireCopy(promptCopy, function () { return promptEl.textContent; });
  }

  selectOS(detectOS());
})();
