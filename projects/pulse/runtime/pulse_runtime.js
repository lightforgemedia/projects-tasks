// Pulse Runtime (v0)
//
// This file is injected into the page (e.g., via Rod's AddInitScript).
// It MUST be deterministic and MUST NOT log by default (no console.*).
//
// API: window.__pulse
// - GetState() -> { url, title, readyState, frameCount }
// - ListInteractive() -> [{ id, role, name, rect, locatorHints, fingerprint }]
// - Act(cmd) -> { ok, type, target, error? }
// - Snapshot(mode) -> { ok, mode, url, title }
(function () {
  if (typeof window === "undefined") return;
  if (window.__pulse) return;

  function normText(s) {
    return String(s || "")
      .replace(/\s+/g, " ")
      .trim();
  }

  function safeIdent(s) {
    return normText(s)
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, "-")
      .replace(/^-+|-+$/g, "")
      .slice(0, 64);
  }

  function rectOf(el) {
    const r = el.getBoundingClientRect();
    return {
      x: Math.round(r.x),
      y: Math.round(r.y),
      w: Math.round(r.width),
      h: Math.round(r.height),
    };
  }

  function locatorHintsFor(el) {
    const hints = [];
    const testid = el.getAttribute && el.getAttribute("data-testid");
    if (testid) hints.push(`data-testid=${testid}`);
    const id = el.id;
    if (id) hints.push(`css=#${id}`);
    const role = el.getAttribute && el.getAttribute("role");
    if (role) hints.push(`role=${role}`);
    return hints;
  }

  function roleOf(el) {
    const roleAttr = el.getAttribute && el.getAttribute("role");
    if (roleAttr) return roleAttr;
    const tag = (el.tagName || "").toLowerCase();
    if (tag === "a") return "link";
    if (tag === "button") return "button";
    if (tag === "input" || tag === "textarea") return "textbox";
    if (tag === "select") return "combobox";
    return tag || "unknown";
  }

  function nameOf(el) {
    const aria = el.getAttribute && el.getAttribute("aria-label");
    if (aria) return normText(aria);
    const testid = el.getAttribute && el.getAttribute("data-testid");
    if (testid) return normText(testid);
    return normText(el.innerText || el.textContent || "");
  }

  function fingerprintOf(el) {
    const role = roleOf(el);
    const name = nameOf(el);
    const testid = el.getAttribute && el.getAttribute("data-testid");
    const basis = testid ? `t:${testid}` : `n:${safeIdent(name)}`;
    return `fp:auto:${role}:${basis}`;
  }

  function selectByTarget(target) {
    const t = String(target || "");
    if (t.startsWith("css=")) {
      return document.querySelector(t.slice("css=".length));
    }
    if (t.startsWith("data-testid=")) {
      const v = t.slice("data-testid=".length);
      return document.querySelector(`[data-testid="${CSS.escape(v)}"]`);
    }
    if (t.startsWith("fp:auto:")) {
      const parts = t.split(":");
      const role = parts[2] || "";
      const basis = parts.slice(3).join(":");
      const candidates = listInteractiveElements();
      for (const c of candidates) {
        if (c.fingerprint === t) return c._el;
      }
      // Fallback: try role-only match (best-effort)
      if (role) {
        for (const c of candidates) {
          if (c.role === role && c.fingerprint.indexOf(basis) !== -1) return c._el;
        }
      }
      return null;
    }
    return null;
  }

  function listInteractiveElements() {
    const els = [];
    const nodes = document.querySelectorAll(
      "button,a,input,select,textarea,[role]"
    );
    for (const el of nodes) {
      if (!el || !el.getBoundingClientRect) continue;
      const r = el.getBoundingClientRect();
      if (!r || r.width <= 0 || r.height <= 0) continue;
      els.push(el);
    }
    return els.map((el, idx) => {
      const role = roleOf(el);
      const name = nameOf(el);
      const rect = rectOf(el);
      const locatorHints = locatorHintsFor(el);
      const fingerprint = fingerprintOf(el);
      return {
        id: `el-${idx}`,
        role,
        name,
        rect,
        locatorHints,
        fingerprint,
        _el: el, // internal only; stripped by ListInteractive()
      };
    });
  }

  window.__pulse = {
    /**
     * @returns {{url:string,title:string,readyState:string,frameCount:number}}
     */
    GetState: function () {
      return {
        url: String(location.href || ""),
        title: String(document.title || ""),
        readyState: String(document.readyState || ""),
        frameCount: (window.frames && window.frames.length) || 0,
      };
    },

    /**
     * @returns {Array<{id:string,role:string,name:string,rect:{x:number,y:number,w:number,h:number},locatorHints:string[],fingerprint:string}>}
     */
    ListInteractive: function () {
      const items = listInteractiveElements();
      // Remove internal element references.
      return items.map((i) => ({
        id: i.id,
        role: i.role,
        name: i.name,
        rect: i.rect,
        locatorHints: i.locatorHints,
        fingerprint: i.fingerprint,
      }));
    },

    /**
     * @param {{type:string,target:string,args?:any}} cmd
     * @returns {{ok:boolean,type:string,target:string,error?:string}}
     */
    Act: function (cmd) {
      const type = (cmd && cmd.type) || "";
      const target = (cmd && cmd.target) || "";
      try {
        if (type !== "click" && type !== "type") {
          return {
            ok: false,
            type: String(type),
            target: String(target),
            error: `unsupported act type: ${String(type)}`,
          };
        }
        const el = selectByTarget(target);
        if (!el) {
          return {
            ok: false,
            type: String(type),
            target: String(target),
            error: "target not found",
          };
        }
        if (type === "click") {
          el.click();
          return { ok: true, type: "click", target: String(target) };
        }
        const value = (cmd && cmd.args && cmd.args.value) || "";
        el.focus();
        el.value = String(value);
        el.dispatchEvent(new Event("input", { bubbles: true }));
        el.dispatchEvent(new Event("change", { bubbles: true }));
        return { ok: true, type: "type", target: String(target) };
      } catch (e) {
        return {
          ok: false,
          type: String(type),
          target: String(target),
          error: String((e && e.message) || e),
        };
      }
    },

    /**
     * @param {"dom"|"accessibility"|"screenshot"|"none"} mode
     * @returns {{ok:boolean,mode:string,url:string,title:string}}
     */
    Snapshot: function (mode) {
      return {
        ok: true,
        mode: String(mode || "none"),
        url: String(location.href || ""),
        title: String(document.title || ""),
      };
    },
  };
})();

