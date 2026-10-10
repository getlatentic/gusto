// Measures the page it is run on against the limits in docs/DESIGN_PRINCIPLES.md. Paste it in the console of a page, or evaluate it
// with a browser tool: it returns one object. A limit that is exceeded is listed in `over`.
(function () {
  var LIMIT = { surfaces: 5, smallest: 12, heaviest: 600, target: 24, contrast: 4.5, nesting: 2 };
  var RADII = [4, 8, 12, 16, 20, 24];
  var SIZES = [12, 13, 14, 16, 20, 24, 28];
  var WEIGHTS = ["400", "500", "600"];
  var upper = [];
  var all = [].slice.call(document.body.querySelectorAll("*")).filter(function (el) {
    var r = el.getBoundingClientRect();
    var s = getComputedStyle(el);
    return r.width > 0 && r.height > 0 && s.visibility !== "hidden" && s.display !== "none" && !el.closest("[hidden],script,style,iframe,svg");
  });
  var px = function (v) { return Math.round(parseFloat(v)); };
  var set = function () { return {}; };
  var radii = set(), sizes = set(), weights = set(), surfaces = set();
  var both = [], wrapped = [], small = [], tiny = [], faint = [], deep = [];

  function parse(c) {
    var m = c.match(/rgba?\(([^)]+)\)/);
    if (!m) { return null; }
    var p = m[1].split(/[ ,\/]+/).filter(Boolean).map(Number);
    return { r: p[0], g: p[1], b: p[2], a: p.length > 3 ? p[3] : 1 };
  }
  function over(top, under) {
    var a = top.a + under.a * (1 - top.a);
    if (!a) { return { r: 0, g: 0, b: 0, a: 0 }; }
    return { r: (top.r * top.a + under.r * under.a * (1 - top.a)) / a, g: (top.g * top.a + under.g * under.a * (1 - top.a)) / a, b: (top.b * top.a + under.b * under.a * (1 - top.a)) / a, a: a };
  }
  function lum(c) {
    var f = function (v) { v /= 255; return v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4); };
    return 0.2126 * f(c.r) + 0.7152 * f(c.g) + 0.0722 * f(c.b);
  }
  function backdrop(el) {
    var layers = [];
    for (var n = el; n && n.nodeType === 1; n = n.parentElement) {
      var c = parse(getComputedStyle(n).backgroundColor);
      if (c && c.a > 0) { layers.push(c); if (c.a === 1) { break; } }
    }
    var base = { r: 255, g: 255, b: 255, a: 1 };
    return layers.reverse().reduce(function (under, top) { return over(top, under); }, base);
  }
  function wrapsText(el) {
    var found = false;
    var walker = document.createTreeWalker(el, NodeFilter.SHOW_TEXT);
    for (var n = walker.nextNode(); n && !found; n = walker.nextNode()) {
      if (!n.textContent.trim()) { continue; }
      var range = document.createRange();
      range.selectNodeContents(n);
      var tops = {};
      [].forEach.call(range.getClientRects(), function (rect) { if (rect.width > 1) { tops[Math.round(rect.top / 3)] = 1; } });
      found = Object.keys(tops).length > 1;
    }
    return found;
  }
  function hasEdge(s) {
    return ["Top", "Right", "Bottom", "Left"].some(function (side) { return parseFloat(s["border" + side + "Width"]) > 0 && s["border" + side + "Style"] !== "none"; });
  }
  function inSentence(el) {
    return [].some.call(el.parentElement.childNodes, function (n) { return n !== el && n.nodeType === 3 && n.textContent.trim().length > 2; });
  }
  function selected(el) { try { return el.matches(":has(input:checked)") || el.matches("[aria-pressed=true],[aria-selected=true]"); } catch (e) { return false; } }
  function hasShadow(s) { return s.boxShadow && s.boxShadow !== "none"; }

  all.forEach(function (el) {
    var s = getComputedStyle(el), r = el.getBoundingClientRect(), label = el.tagName.toLowerCase() + (el.className && typeof el.className === "string" ? "." + el.className.trim().split(/\s+/)[0] : "");
    var rad = parseFloat(s.borderTopLeftRadius);
    if (rad > 0 && rad < 200 && r.width > 24 && r.height > 24 && Math.min(r.width, r.height) / 2 > rad + 0.5) { radii[px(rad)] = (radii[px(rad)] || 0) + 1; }
    var own = [].some.call(el.childNodes, function (n) { return n.nodeType === 3 && n.textContent.trim(); });
    if (own) {
      sizes[px(s.fontSize)] = (sizes[px(s.fontSize)] || 0) + 1;
      weights[s.fontWeight] = (weights[s.fontWeight] || 0) + 1;
      if (px(s.fontSize) < LIMIT.smallest) { tiny.push(label + " " + px(s.fontSize) + "px"); }
      if (s.textTransform === "uppercase") { upper.push(label); }
      var fg = parse(s.color), bg = backdrop(el);
      if (fg) {
        var f = over(fg, bg), l1 = lum(f), l2 = lum(bg), ratio = (Math.max(l1, l2) + 0.05) / (Math.min(l1, l2) + 0.05);
        var big = px(s.fontSize) >= 24 || (px(s.fontSize) >= 18.66 && parseInt(s.fontWeight, 10) >= 700);
        if (ratio < (big ? 3 : LIMIT.contrast)) { faint.push(label + " " + ratio.toFixed(2)); }
      }
    }
    var bgc = parse(s.backgroundColor);
    if (bgc && bgc.a > 0.9 && r.width * r.height > 20000) { var key = "rgb(" + bgc.r + "," + bgc.g + "," + bgc.b + ")"; surfaces[key] = (surfaces[key] || 0) + 1; }
    if (hasEdge(s) && hasShadow(s) && !/^(input|textarea|select|button)$/.test(el.tagName.toLowerCase()) && !/(^| )(box|menu|dialog|dialog-card|popover)( |$)/.test(el.className || "") && !selected(el) && r.width > 40 && r.height > 24) { both.push(label); }
    if (/^(button|a)$/.test(el.tagName.toLowerCase()) || el.getAttribute("role") === "button") {
      if (el.textContent.trim().length <= 40 && wrapsText(el)) { wrapped.push(label + ' "' + el.textContent.trim().slice(0, 24) + '"'); }
      if (r.height < LIMIT.target || (r.width < LIMIT.target && el.textContent.trim().length < 2 && !el.closest("p"))) { if (!(el.tagName === "A" && (s.display === "inline" || inSentence(el)))) { small.push(label + " " + Math.round(r.width) + "x" + Math.round(r.height)); } }
    }
    var depth = 0;
    for (var n = el.parentElement; n && n !== document.body; n = n.parentElement) {
      var ns = getComputedStyle(n);
      if ((hasEdge(ns) || hasShadow(ns)) && parseFloat(ns.borderTopLeftRadius) > 0) { depth++; }
    }
    if ((hasEdge(s) || hasShadow(s)) && parseFloat(s.borderTopLeftRadius) > 0 && depth + 1 > LIMIT.nesting && r.width > 40) { deep.push(label + " depth " + (depth + 1)); }
  });

  var count = function (o) { return Object.keys(o).length; };
  var offScale = function (o, scale) { return Object.keys(o).filter(function (k) { return scale.map(String).indexOf(k) < 0; }); };
  var badRadii = offScale(radii, RADII), badSizes = offScale(sizes, SIZES), badWeights = offScale(weights, WEIGHTS);
  var report = {
    page: location.pathname,
    radiiUsed: Object.keys(radii).map(Number).sort(function (a, b) { return a - b; }),
    fontSizes: Object.keys(sizes).map(Number).sort(function (a, b) { return a - b; }),
    fontWeights: Object.keys(weights).sort(),
    surfaces: Object.keys(surfaces),
    borderAndShadow: both.slice(0, 8),
    nestedOutlines: deep.slice(0, 8),
    tinyText: tiny.slice(0, 8),
    allCaps: upper.slice(0, 8),
    wrappedLabels: wrapped.slice(0, 8),
    smallTargets: small.slice(0, 8),
    lowContrast: faint.slice(0, 8)
  };
  report.over = [];
  if (badRadii.length) { report.over.push("radius off the scale: " + badRadii.join(",")); }
  if (badSizes.length) { report.over.push("font size off the scale: " + badSizes.join(",")); }
  if (badWeights.length) { report.over.push("font weight off the scale: " + badWeights.join(",")); }
  if (count(surfaces) > LIMIT.surfaces) { report.over.push("surfaces " + count(surfaces) + " > " + LIMIT.surfaces); }
  ["borderAndShadow", "nestedOutlines", "tinyText", "allCaps", "wrappedLabels", "smallTargets", "lowContrast"].forEach(function (k) { if (report[k].length) { report.over.push(k + " " + report[k].length); } });
  return report;
})();
