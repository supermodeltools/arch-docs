function toSlug(s) { return s.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, ""); }

window.addEventListener("load", function() {

  // --- Architecture Map ---
  var archDataEl = document.getElementById("arch-map-data");
  var archEl = document.getElementById("arch-map-container");
  if (archDataEl && archEl) {
    try {
      var raw = archDataEl.textContent.trim();
      var data = JSON.parse(raw);
      if (typeof data === "string") data = JSON.parse(data);
      var svgContainer = archEl.querySelector(".arch-map-svg");
      if (data && svgContainer) {
        var items = [];
        if (data.domain) items.push(data.domain);
        if (data.subdomain) items.push(data.subdomain);
        if (data.file) items.push(data.file);
        if (data.entity) items.push(data.entity);

        if (items.length > 1) {
          var boxW = 140, boxH = 36, arrowW = 28, pad = 12;
          var totalW = items.length * boxW + (items.length - 1) * arrowW + pad * 2;
          var totalH = boxH + pad * 2;
          var svg = '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ' + totalW + ' ' + totalH + '" style="max-height:60px">';

          for (var i = 0; i < items.length; i++) {
            var x = pad + i * (boxW + arrowW);
            var y = pad;
            var isLast = i === items.length - 1;
            var fill = isLast ? "#6366f1" : "#1a1d27";
            var stroke = isLast ? "#818cf8" : "#2a2e3e";
            var textColor = isLast ? "#fff" : "#e4e4e7";
            var label = items[i].name || "";
            if (label.length > 16) label = label.substring(0, 14) + "..";

            if (items[i].slug && !isLast) {
              svg += '<a href="/' + items[i].slug + '.html">';
            }
            svg += '<rect x="' + x + '" y="' + y + '" width="' + boxW + '" height="' + boxH + '" rx="6" fill="' + fill + '" stroke="' + stroke + '" stroke-width="1"/>';
            svg += '<text x="' + (x + boxW / 2) + '" y="' + (y + boxH / 2 + 5) + '" text-anchor="middle" fill="' + textColor + '" font-size="12" font-family="Inter,system-ui,sans-serif">' + label + '</text>';
            if (items[i].slug && !isLast) {
              svg += '</a>';
            }

            if (i < items.length - 1) {
              var ax = x + boxW + 4;
              var ay = y + boxH / 2;
              svg += '<path d="M' + ax + ' ' + ay + ' L' + (ax + arrowW - 8) + ' ' + ay + '" stroke="#2a2e3e" stroke-width="1.5" fill="none"/>';
              svg += '<polygon points="' + (ax + arrowW - 8) + ',' + (ay - 4) + ' ' + (ax + arrowW - 2) + ',' + ay + ' ' + (ax + arrowW - 8) + ',' + (ay + 4) + '" fill="#2a2e3e"/>';
            }
          }

          svg += '</svg>';
          svgContainer.innerHTML = svg;
        }
      }
    } catch (e) {
      console.error("Architecture map error:", e);
    }
  }

  // --- Force-Directed Graph (D3) — enriched nodes ---
  var graphDataEl = document.getElementById("graph-data");
  var graphEl = document.getElementById("force-graph");
  if (graphDataEl && graphEl && typeof d3 !== "undefined") {
    try {
      var rawGraph = graphDataEl.textContent.trim();
      var graphData = JSON.parse(rawGraph);
      if (typeof graphData === "string") graphData = JSON.parse(graphData);
      var centerSlug = graphEl.getAttribute("data-center");

      if (graphData && graphData.nodes && graphData.nodes.length > 1) {
        var width = graphEl.clientWidth || 600;
        var height = 420;

        var typeColors = {
          File: "#3b82f6", Function: "#22c55e", Class: "#f59e0b",
          Type: "#ef4444", Domain: "#6366f1", Subdomain: "#a855f7", Directory: "#6b7280"
        };
        var edgeColors = {
          imports: "#3b82f6", calls: "#22c55e", defines: "#f59e0b",
          extends: "#ef4444", contains: "#6b7280", belongsTo: "#a855f7", partOf: "#6366f1"
        };

        // Compute node radius from enriched lineCount data
        var maxLC = d3.max(graphData.nodes, function(d) { return d.lc || 0; }) || 1;
        var rScale = d3.scaleSqrt().domain([0, maxLC]).range([6, 22]);
        function nodeR(d) {
          if (d.slug === centerSlug) return Math.max(rScale(d.lc || 0), 14);
          if (d.lc > 0) return rScale(d.lc);
          return 7;
        }

        var svg = d3.select(graphEl).append("svg").attr("width", width).attr("height", height);

        // Edge type legend at top
        var legendTypes = {};
        graphData.edges.forEach(function(e) { legendTypes[e.type] = true; });
        var legendKeys = Object.keys(legendTypes);
        var lgX = 4;
        legendKeys.forEach(function(t) {
          svg.append("rect").attr("x", lgX).attr("y", 4).attr("width", 10).attr("height", 10).attr("rx", 2)
            .attr("fill", edgeColors[t] || "#2a2e3e");
          svg.append("text").attr("x", lgX + 14).attr("y", 12).attr("fill", "#6b7280").attr("font-size", "10px")
            .attr("font-family", "Inter,system-ui,sans-serif").text(t);
          lgX += t.length * 6 + 26;
        });

        var simulation = d3.forceSimulation(graphData.nodes)
          .force("link", d3.forceLink(graphData.edges).id(function(d) { return d.id; }).distance(90))
          .force("charge", d3.forceManyBody().strength(-250))
          .force("center", d3.forceCenter(width / 2, height / 2 + 10))
          .force("collision", d3.forceCollide().radius(function(d) { return nodeR(d) + 8; }));

        var link = svg.append("g").selectAll("line").data(graphData.edges).enter().append("line")
          .attr("stroke", function(d) { return edgeColors[d.type] || "#2a2e3e"; })
          .attr("stroke-opacity", 0.6).attr("stroke-width", 1.5);

        var node = svg.append("g").selectAll("g").data(graphData.nodes).enter().append("g")
          .style("cursor", function(d) { return d.slug ? "pointer" : "default"; })
          .call(d3.drag()
            .on("start", function(event, d) {
              if (!event.active) simulation.alphaTarget(0.3).restart();
              d.fx = d.x; d.fy = d.y;
            })
            .on("drag", function(event, d) { d.fx = event.x; d.fy = event.y; })
            .on("end", function(event, d) {
              if (!event.active) simulation.alphaTarget(0);
              d.fx = null; d.fy = null;
            })
          );

        node.append("circle")
          .attr("r", nodeR)
          .attr("fill", function(d) { return typeColors[d.type] || "#6b7280"; })
          .attr("stroke", function(d) { return d.slug === centerSlug ? "#fff" : "none"; })
          .attr("stroke-width", function(d) { return d.slug === centerSlug ? 2.5 : 0; })
          .attr("opacity", function(d) { return d.slug === centerSlug ? 1 : 0.85; });

        // Show line count inside larger nodes
        node.filter(function(d) { return d.lc > 0 && nodeR(d) >= 14; }).append("text")
          .text(function(d) { return d.lc; })
          .attr("text-anchor", "middle").attr("y", 4).attr("fill", "#fff")
          .attr("font-size", "9px").attr("font-weight", "600")
          .attr("font-family", "Inter,system-ui,sans-serif");

        node.append("text")
          .text(function(d) { var l = d.label || ""; return l.length > 22 ? l.substring(0, 20) + ".." : l; })
          .attr("x", 0)
          .attr("y", function(d) { return -(nodeR(d) + 4); })
          .attr("text-anchor", "middle").attr("fill", "#9ca3af")
          .attr("font-size", "11px").attr("font-family", "Inter,system-ui,sans-serif");

        // Enriched tooltip
        node.append("title").text(function(d) {
          var parts = [d.label, d.type];
          if (d.lang) parts.push(d.lang);
          if (d.lc) parts.push(d.lc + " lines");
          if (d.cc) parts.push("calls " + d.cc);
          if (d.cbc) parts.push("called by " + d.cbc);
          return parts.join(" · ");
        });

        node.on("click", function(event, d) {
          if (d.slug) window.location.href = "/" + d.slug + ".html";
        });

        simulation.on("tick", function() {
          link.attr("x1", function(d) { return d.source.x; }).attr("y1", function(d) { return d.source.y; })
              .attr("x2", function(d) { return d.target.x; }).attr("y2", function(d) { return d.target.y; });
          node.attr("transform", function(d) {
            d.x = Math.max(24, Math.min(width - 24, d.x));
            d.y = Math.max(24, Math.min(height - 24, d.y));
            return "translate(" + d.x + "," + d.y + ")";
          });
        });
      }
    } catch (e) {
      console.error("Force graph error:", e);
    }
  }

  // --- Entity Profile Chart (compact format) ---
  var epDataEl = document.getElementById("entity-profile-data");
  var epChartEl = document.getElementById("entity-profile-chart");
  if (epDataEl && epChartEl && typeof d3 !== "undefined") {
    try {
      var ep = JSON.parse(epDataEl.textContent.trim());
      var epW = epChartEl.clientWidth || 700;

      // Build metrics from compact keys
      var metricDefs = [
        { key: "lc", label: "Lines of Code", color: "#6366f1" },
        { key: "co", label: "Calls Out", color: "#3b82f6" },
        { key: "cb", label: "Called By", color: "#22c55e" },
        { key: "ic", label: "Imports", color: "#f59e0b" },
        { key: "ib", label: "Imported By", color: "#a855f7" },
        { key: "fn", label: "Functions", color: "#ec4899" },
        { key: "cl", label: "Classes", color: "#ef4444" },
        { key: "tc", label: "Types", color: "#f97316" },
        { key: "fc", label: "Files", color: "#6b7280" }
      ];
      var metrics = metricDefs.filter(function(d) { return ep[d.key] > 0; })
        .map(function(d) { return { label: d.label, value: ep[d.key], color: d.color }; });

      // Edge types from compact map {type: count}
      var et = ep.et || {};
      var edgeTypes = Object.keys(et).map(function(k) { return { type: k, count: et[k] }; })
        .sort(function(a, b) { return b.count - a.count; });

      var epEdgeColors = {
        calls: "#3b82f6", defines: "#22c55e", belongsTo: "#a855f7",
        imports: "#f59e0b", extends: "#ef4444", contains: "#6b7280", partOf: "#6366f1"
      };

      var hasMetrics = metrics.length > 0;
      var hasEdges = edgeTypes.length > 0;
      var metricsH = hasMetrics ? metrics.length * 32 + 8 : 0;
      var edgesH = hasEdges ? Math.max(edgeTypes.length * 22 + 40, 56) : 0;
      var fileBarH = (ep.sl > 0 && ep.el > 0) ? 44 : 0;
      var totalH = metricsH + edgesH + fileBarH + 4;
      if (totalH < 40) totalH = 40;

      var svg = d3.select(epChartEl).append("svg").attr("width", epW).attr("height", totalH);
      var yOff = 0;

      if (hasMetrics) {
        var maxVal = d3.max(metrics, function(d) { return d.value; }) || 1;
        var labelW = 100;
        var barMaxW = Math.min(epW - labelW - 70, 400);
        var barScale = d3.scaleLinear().domain([0, maxVal]).range([0, barMaxW]);
        metrics.forEach(function(m, i) {
          var y = yOff + i * 32 + 4;
          svg.append("text").attr("x", labelW - 6).attr("y", y + 13).attr("text-anchor", "end")
            .attr("fill", "#9ca3af").attr("font-size", "12px").attr("font-family", "Inter,system-ui,sans-serif").text(m.label);
          svg.append("rect").attr("x", labelW).attr("y", y).attr("width", Math.max(barScale(m.value), 4)).attr("height", 20)
            .attr("rx", 3).attr("fill", m.color).attr("opacity", 0.85);
          svg.append("text").attr("x", labelW + Math.max(barScale(m.value), 4) + 6).attr("y", y + 14)
            .attr("fill", "#e4e4e7").attr("font-size", "13px").attr("font-weight", "600")
            .attr("font-family", "Inter,system-ui,sans-serif").text(m.value);
        });
        yOff += metricsH;
      }

      if (hasEdges) {
        var totalEdgeCount = edgeTypes.reduce(function(s, d) { return s + d.count; }, 0);
        var stackW = Math.min(epW - 130, 500);
        var stackScale = d3.scaleLinear().domain([0, totalEdgeCount]).range([0, stackW]);
        var sx = 100, sy = yOff + 6;
        svg.append("text").attr("x", 0).attr("y", sy + 2).attr("fill", "#6b7280").attr("font-size", "11px")
          .attr("font-weight", "600").attr("font-family", "Inter,system-ui,sans-serif").text("RELATIONSHIPS");
        var cx = sx;
        edgeTypes.forEach(function(e, i) {
          var w = Math.max(stackScale(e.count), 3);
          svg.append("rect").attr("x", cx).attr("y", sy - 6).attr("width", w).attr("height", 18)
            .attr("rx", i === 0 ? 3 : 0).attr("fill", epEdgeColors[e.type] || "#6b7280").attr("opacity", 0.85);
          cx += w;
        });
        var ly = sy + 18, lx = sx;
        edgeTypes.forEach(function(e) {
          svg.append("rect").attr("x", lx).attr("y", ly).attr("width", 8).attr("height", 8).attr("rx", 2)
            .attr("fill", epEdgeColors[e.type] || "#6b7280");
          svg.append("text").attr("x", lx + 12).attr("y", ly + 7).attr("fill", "#9ca3af").attr("font-size", "10px")
            .attr("font-family", "Inter,system-ui,sans-serif").text(e.type + " " + e.count);
          lx += e.type.length * 6.5 + 36;
          if (lx > epW - 60) { lx = sx; ly += 16; }
        });
        yOff += edgesH;
      }

      if (ep.sl > 0 && ep.el > 0) {
        var fy = yOff + 8, fw = Math.min(epW - 130, 500), fx = 100;
        svg.append("text").attr("x", 0).attr("y", fy + 2).attr("fill", "#6b7280").attr("font-size", "11px")
          .attr("font-weight", "600").attr("font-family", "Inter,system-ui,sans-serif").text("FILE POSITION");
        svg.append("rect").attr("x", fx).attr("y", fy - 5).attr("width", fw).attr("height", 14).attr("rx", 3)
          .attr("fill", "#1a1d27").attr("stroke", "#2a2e3e").attr("stroke-width", 1);
        var est = Math.max(ep.el * 1.15, ep.el + 20);
        var hx = fx + (ep.sl / est) * fw, hw = Math.max(((ep.el - ep.sl) / est) * fw, 3);
        svg.append("rect").attr("x", hx).attr("y", fy - 5).attr("width", hw).attr("height", 14).attr("rx", 2)
          .attr("fill", "#6366f1").attr("opacity", 0.8);
        svg.append("text").attr("x", fx + fw + 6).attr("y", fy + 4).attr("fill", "#9ca3af").attr("font-size", "10px")
          .attr("font-family", "Inter,system-ui,sans-serif").text("L" + ep.sl + "–" + ep.el);
      }
    } catch (e) { console.error("Entity profile chart error:", e); }
  }

  // --- Architecture Overview (Homepage Force Graph) ---
  var archOverDataEl = document.getElementById("arch-overview-data");
  var archOverEl = document.getElementById("arch-overview");
  if (archOverDataEl && archOverEl && typeof d3 !== "undefined") {
    try {
      var archData = JSON.parse(archOverDataEl.textContent.trim());
      if (archData && archData.nodes && archData.nodes.length > 1) {
        var aoW = archOverEl.clientWidth || 800;
        var aoH = 420;
        var aoTypeColors = { root: "#6366f1", domain: "#3b82f6", subdomain: "#a855f7" };
        var aoSvg = d3.select(archOverEl).append("svg").attr("width", aoW).attr("height", aoH);

        var maxCount = d3.max(archData.nodes, function(d) { return d.count; }) || 1;
        var radiusScale = d3.scaleSqrt().domain([0, maxCount]).range([8, 36]);

        var aoSim = d3.forceSimulation(archData.nodes)
          .force("link", d3.forceLink(archData.links).id(function(d) { return d.id; }).distance(function(d) {
            return d.source.type === "root" || d.source === "root" ? 140 : 90;
          }))
          .force("charge", d3.forceManyBody().strength(-300))
          .force("center", d3.forceCenter(aoW / 2, aoH / 2))
          .force("collision", d3.forceCollide().radius(function(d) { return radiusScale(d.count) + 12; }));

        var aoLink = aoSvg.append("g").selectAll("line").data(archData.links).enter().append("line")
          .attr("stroke", "#2a2e3e").attr("stroke-opacity", 0.6).attr("stroke-width", 1.5);

        var aoNode = aoSvg.append("g").selectAll("g").data(archData.nodes).enter().append("g")
          .style("cursor", function(d) { return d.slug ? "pointer" : "default"; })
          .call(d3.drag()
            .on("start", function(event, d) {
              if (!event.active) aoSim.alphaTarget(0.3).restart();
              d.fx = d.x; d.fy = d.y;
            })
            .on("drag", function(event, d) { d.fx = event.x; d.fy = event.y; })
            .on("end", function(event, d) {
              if (!event.active) aoSim.alphaTarget(0);
              d.fx = null; d.fy = null;
            })
          );

        aoNode.append("circle")
          .attr("r", function(d) { return d.type === "root" ? 24 : radiusScale(d.count); })
          .attr("fill", function(d) { return aoTypeColors[d.type] || "#6b7280"; })
          .attr("opacity", 0.9)
          .attr("stroke", function(d) { return d.type === "root" ? "#818cf8" : "none"; })
          .attr("stroke-width", function(d) { return d.type === "root" ? 2 : 0; });

        aoNode.append("text")
          .text(function(d) { var l = d.name; return l.length > 20 ? l.substring(0, 18) + ".." : l; })
          .attr("x", 0)
          .attr("y", function(d) { return (d.type === "root" ? 24 : radiusScale(d.count)) + 14; })
          .attr("text-anchor", "middle").attr("fill", "#9ca3af")
          .attr("font-size", function(d) { return d.type === "root" ? "13px" : "11px"; })
          .attr("font-weight", function(d) { return d.type === "root" ? "600" : "400"; })
          .attr("font-family", "Inter,system-ui,sans-serif");

        aoNode.filter(function(d) { return d.type !== "root" && d.count > 0; }).append("text")
          .text(function(d) { return d.count; })
          .attr("text-anchor", "middle").attr("y", 4).attr("fill", "#fff")
          .attr("font-size", "11px").attr("font-weight", "600")
          .attr("font-family", "Inter,system-ui,sans-serif");

        aoNode.on("click", function(event, d) {
          if (d.slug) window.location.href = "/" + d.slug + ".html";
        });

        aoNode.append("title").text(function(d) {
          return d.name + (d.count ? " (" + d.count + " entities)" : "");
        });

        aoSim.on("tick", function() {
          aoLink.attr("x1", function(d) { return d.source.x; }).attr("y1", function(d) { return d.source.y; })
                .attr("x2", function(d) { return d.target.x; }).attr("y2", function(d) { return d.target.y; });
          aoNode.attr("transform", function(d) {
            d.x = Math.max(40, Math.min(aoW - 40, d.x));
            d.y = Math.max(40, Math.min(aoH - 40, d.y));
            return "translate(" + d.x + "," + d.y + ")";
          });
        });
      }
    } catch (e) { console.error("Architecture overview error:", e); }
  }

  // --- Homepage Treemap (clickable) ---
  var hpDataEl = document.getElementById("homepage-chart-data");
  var hpChartEl = document.getElementById("homepage-chart");
  if (hpDataEl && hpChartEl && typeof d3 !== "undefined") {
    try {
      var hpData = JSON.parse(hpDataEl.textContent.trim());
      var hpW = hpChartEl.clientWidth || 800;
      var hpH = 300;
      var children = (hpData.taxonomies || []).map(function(t) {
        return { name: t.name, value: t.count, slug: t.slug };
      });
      if (children.length > 0) {
        var root = d3.hierarchy({ name: "root", children: children }).sum(function(d) { return d.value || 0; }).sort(function(a, b) { return b.value - a.value; });
        d3.treemap().size([hpW, hpH]).padding(3).round(true)(root);
        var colors = ["#6366f1", "#3b82f6", "#22c55e", "#f59e0b", "#ef4444", "#a855f7", "#ec4899", "#6b7280"];
        var svg = d3.select(hpChartEl).append("svg").attr("width", hpW).attr("height", hpH);
        var cell = svg.selectAll("g").data(root.leaves()).enter().append("g")
          .attr("transform", function(d) { return "translate(" + d.x0 + "," + d.y0 + ")"; })
          .style("cursor", "pointer")
          .on("click", function(event, d) { if (d.data.slug) window.location.href = "/" + d.data.slug + "/index.html"; });
        cell.append("rect").attr("width", function(d) { return d.x1 - d.x0; }).attr("height", function(d) { return d.y1 - d.y0; }).attr("rx", 4).attr("fill", function(d, i) { return colors[i % colors.length]; }).attr("opacity", 0.85);
        cell.append("text").attr("x", 8).attr("y", 20).attr("fill", "#fff").attr("font-size", "13px").attr("font-weight", "600").attr("font-family", "Inter,system-ui,sans-serif").text(function(d) { var w = d.x1 - d.x0; return w > 60 ? d.data.name : ""; });
        cell.append("text").attr("x", 8).attr("y", 38).attr("fill", "rgba(255,255,255,0.7)").attr("font-size", "12px").attr("font-family", "Inter,system-ui,sans-serif").text(function(d) { var w = d.x1 - d.x0; return w > 50 ? d.data.value : ""; });
        cell.append("title").text(function(d) { return d.data.name + ": " + d.data.value + " entries"; });
      }
    } catch (e) { console.error("Homepage chart error:", e); }
  }

  // --- Hub Charts (donut + top entities) ---
  var hubDataEl = document.getElementById("hub-chart-data");
  var hubChartEl = document.getElementById("hub-chart");
  var hubSecEl = document.getElementById("hub-chart-secondary");
  if (hubDataEl && hubChartEl && typeof d3 !== "undefined") {
    try {
      var hubData = JSON.parse(hubDataEl.textContent.trim());
      var distributions = hubData.distributions || {};
      var hubColors = ["#6366f1", "#3b82f6", "#22c55e", "#f59e0b", "#ef4444", "#a855f7", "#ec4899", "#6b7280"];
      var dimLabels = { node_type: "Node Types", language: "Languages", domain: "Domains", extension: "File Extensions" };
      var dimOrder = ["node_type", "language", "domain", "extension"];

      // Pick the distribution with the most entries (>1 entry preferred)
      var bestKey = null;
      var bestLen = 0;
      dimOrder.forEach(function(key) {
        var arr = distributions[key] || [];
        if (arr.length > bestLen) { bestLen = arr.length; bestKey = key; }
      });

      var dist = bestKey ? (distributions[bestKey] || []) : [];

      // LEFT: Donut or profile bars
      if (dist.length > 1) {
        var hubW = hubChartEl.clientWidth || 400;
        var hubH = 220;
        var radius = Math.min(hubW * 0.3, hubH * 0.42);
        var innerR = radius * 0.55;
        var pie = d3.pie().value(function(d) { return d.count; }).sort(null);
        var arc = d3.arc().innerRadius(innerR).outerRadius(radius);
        var svg = d3.select(hubChartEl).append("svg").attr("width", hubW).attr("height", hubH);
        var cx = Math.min(hubH / 2 + 10, hubW * 0.3);
        var g = svg.append("g").attr("transform", "translate(" + cx + "," + (hubH / 2) + ")");
        var arcs = g.selectAll("path").data(pie(dist)).enter().append("path").attr("d", arc).attr("fill", function(d, i) { return hubColors[i % hubColors.length]; }).attr("stroke", "#0f1117").attr("stroke-width", 2).style("cursor", "pointer")
          .on("click", function(event, d) { window.location.href = "/" + bestKey + "/" + toSlug(d.data.name) + ".html"; });
        arcs.append("title").text(function(d) { return d.data.name + ": " + d.data.count; });
        g.append("text").attr("text-anchor", "middle").attr("y", 6).attr("fill", "#e4e4e7").attr("font-size", "20px").attr("font-weight", "700").attr("font-family", "Inter,system-ui,sans-serif").text(hubData.totalEntities || "");
        svg.append("text").attr("x", cx).attr("y", hubH - 4).attr("text-anchor", "middle").attr("fill", "#6b7280").attr("font-size", "11px").attr("font-family", "Inter,system-ui,sans-serif").text(dimLabels[bestKey] || bestKey);
        var legendX = cx + radius + 20;
        dist.forEach(function(d, i) {
          if (i >= 8) return;
          var ly = 16 + i * 22;
          var lg = svg.append("g").style("cursor", "pointer").on("click", function() { window.location.href = "/" + bestKey + "/" + toSlug(d.name) + ".html"; });
          lg.append("rect").attr("x", legendX).attr("y", ly).attr("width", 10).attr("height", 10).attr("rx", 2).attr("fill", hubColors[i % hubColors.length]);
          lg.append("text").attr("x", legendX + 16).attr("y", ly + 9).attr("fill", "#9ca3af").attr("font-size", "11px").attr("font-family", "Inter,system-ui,sans-serif").text(d.name + " (" + d.count + ")");
        });
      } else {
        var profileBars = [];
        dimOrder.forEach(function(key) {
          var arr = distributions[key] || [];
          if (arr.length > 0) {
            profileBars.push({ name: dimLabels[key] || key, count: arr.length, detail: arr.map(function(d) { return d.name; }).join(", ") });
          }
        });
        if (profileBars.length > 0) {
          var pbW = hubChartEl.clientWidth || 400;
          var pbBarH = 28;
          var pbGap = 5;
          var pbH = profileBars.length * (pbBarH + pbGap) + 30;
          var pbLabelW = 120;
          var pbMax = d3.max(profileBars, function(d) { return d.count; }) || 1;
          var pbScale = d3.scaleLinear().domain([0, pbMax]).range([0, pbW - pbLabelW - 100]);
          var svg = d3.select(hubChartEl).append("svg").attr("width", pbW).attr("height", pbH);
          svg.append("text").attr("x", 0).attr("y", 14).attr("fill", "#6b7280").attr("font-size", "11px").attr("font-family", "Inter,system-ui,sans-serif").text(hubData.entryName + " — " + hubData.totalEntities + " entities");
          profileBars.forEach(function(d, i) {
            var y = 24 + i * (pbBarH + pbGap);
            svg.append("text").attr("x", pbLabelW - 6).attr("y", y + pbBarH / 2 + 4).attr("text-anchor", "end").attr("fill", "#9ca3af").attr("font-size", "12px").attr("font-family", "Inter,system-ui,sans-serif").text(d.name);
            svg.append("rect").attr("x", pbLabelW).attr("y", y).attr("width", Math.max(pbScale(d.count), 4)).attr("height", pbBarH).attr("rx", 3).attr("fill", hubColors[i % hubColors.length]).attr("opacity", 0.85);
            svg.append("text").attr("x", pbLabelW + Math.max(pbScale(d.count), 4) + 6).attr("y", y + pbBarH / 2 + 4).attr("fill", "#e4e4e7").attr("font-size", "12px").attr("font-weight", "600").attr("font-family", "Inter,system-ui,sans-serif").text(d.detail);
          });
        }
      }

      // RIGHT: Top entities by line count
      var topEnts = hubData.topEntities || [];
      if (hubSecEl && topEnts.length > 0) {
        var teW = hubSecEl.clientWidth || 400;
        var teBarH = 22;
        var teGap = 3;
        var teH = topEnts.length * (teBarH + teGap) + 24;
        var teLabelW = Math.min(teW * 0.45, 200);
        var teMax = d3.max(topEnts, function(d) { return d.lines; }) || 1;
        var teScale = d3.scaleLinear().domain([0, teMax]).range([0, teW - teLabelW - 60]);
        var typeColors = { Function: "#22c55e", Class: "#f59e0b", File: "#3b82f6", Type: "#ef4444", Domain: "#6366f1", Subdomain: "#a855f7" };

        var teSvg = d3.select(hubSecEl).append("svg").attr("width", teW).attr("height", teH);
        teSvg.append("text").attr("x", 0).attr("y", 12).attr("fill", "#6b7280").attr("font-size", "11px").attr("font-weight", "600")
          .attr("text-transform", "uppercase").attr("letter-spacing", "0.04em")
          .attr("font-family", "Inter,system-ui,sans-serif").text("LARGEST BY LINES OF CODE");

        topEnts.forEach(function(d, i) {
          var y = 22 + i * (teBarH + teGap);
          var label = d.name.replace(/ — .*/, "");
          if (label.length > 26) label = label.substring(0, 24) + "..";
          var g = teSvg.append("g").style("cursor", "pointer")
            .on("click", function() { window.location.href = "/" + d.slug + ".html"; });
          g.append("text").attr("x", teLabelW - 6).attr("y", y + teBarH / 2 + 4).attr("text-anchor", "end")
            .attr("fill", "#9ca3af").attr("font-size", "11px").attr("font-family", "Inter,system-ui,sans-serif").text(label);
          g.append("rect").attr("x", teLabelW).attr("y", y).attr("width", Math.max(teScale(d.lines), 3)).attr("height", teBarH)
            .attr("rx", 3).attr("fill", typeColors[d.type] || "#6366f1").attr("opacity", 0.85);
          g.append("text").attr("x", teLabelW + Math.max(teScale(d.lines), 3) + 5).attr("y", y + teBarH / 2 + 4)
            .attr("fill", "#6b7280").attr("font-size", "10px").attr("font-family", "Inter,system-ui,sans-serif").text(d.lines);
          g.append("title").text(d.name + " (" + d.type + ") — " + d.lines + " lines");
        });
      }
    } catch (e) { console.error("Hub chart error:", e); }
  }

  // --- Taxonomy Index Bar Chart ---
  var taxDataEl = document.getElementById("taxonomy-chart-data");
  var taxChartEl = document.getElementById("taxonomy-chart");
  if (taxDataEl && taxChartEl && typeof d3 !== "undefined") {
    try {
      var taxData = JSON.parse(taxDataEl.textContent.trim());
      var entries = (taxData.entries || []).slice(0, 20);
      var taxKey = taxData.taxonomyKey || "";
      if (entries.length > 0) {
        var taxW = taxChartEl.clientWidth || 800;
        var barH = 26;
        var gap = 4;
        var taxH = entries.length * (barH + gap);
        var labelW = 180;
        var maxCount = d3.max(entries, function(d) { return d.count; }) || 1;
        var barScale = d3.scaleLinear().domain([0, maxCount]).range([0, taxW - labelW - 80]);
        var svg = d3.select(taxChartEl).append("svg").attr("width", taxW).attr("height", taxH);
        entries.forEach(function(d, i) {
          var y = i * (barH + gap);
          var label = d.name.length > 22 ? d.name.substring(0, 20) + ".." : d.name;
          var g = svg.append("g").style("cursor", "pointer").on("click", function() { if (taxKey) window.location.href = "/" + taxKey + "/" + toSlug(d.name) + ".html"; });
          g.append("text").attr("x", labelW - 8).attr("y", y + barH / 2 + 4).attr("text-anchor", "end").attr("fill", "#9ca3af").attr("font-size", "13px").attr("font-family", "Inter,system-ui,sans-serif").text(label);
          g.append("rect").attr("x", labelW).attr("y", y).attr("width", Math.max(barScale(d.count), 4)).attr("height", barH).attr("rx", 3).attr("fill", "#6366f1").attr("opacity", 0.85);
          g.append("text").attr("x", labelW + Math.max(barScale(d.count), 4) + 8).attr("y", y + barH / 2 + 4).attr("fill", "#9ca3af").attr("font-size", "12px").attr("font-family", "Inter,system-ui,sans-serif").text(d.count);
        });
      }
    } catch (e) { console.error("Taxonomy chart error:", e); }
  }

  // --- All Entities Packed Circles ---
  var aeDataEl = document.getElementById("all-entities-chart-data");
  var aeChartEl = document.getElementById("all-entities-chart");
  if (aeDataEl && aeChartEl && typeof d3 !== "undefined") {
    try {
      var aeData = JSON.parse(aeDataEl.textContent.trim());
      var types = aeData.typeDistribution || [];
      if (types.length > 0) {
        var aeW = aeChartEl.clientWidth || 800;
        var aeH = 320;
        var aeColors = ["#6366f1", "#3b82f6", "#22c55e", "#f59e0b", "#ef4444", "#a855f7", "#ec4899", "#6b7280"];
        var root = d3.hierarchy({ children: types }).sum(function(d) { return d.count || 0; });
        d3.pack().size([aeW, aeH]).padding(4)(root);
        var svg = d3.select(aeChartEl).append("svg").attr("width", aeW).attr("height", aeH);
        var node = svg.selectAll("g").data(root.leaves()).enter().append("g").attr("transform", function(d) { return "translate(" + d.x + "," + d.y + ")"; })
          .style("cursor", "pointer").on("click", function(event, d) { window.location.href = "/" + "node_type/" + toSlug(d.data.name) + ".html"; });
        node.append("circle").attr("r", function(d) { return d.r; }).attr("fill", function(d, i) { return aeColors[i % aeColors.length]; }).attr("opacity", 0.8).attr("stroke", "#0f1117").attr("stroke-width", 1);
        node.append("text").attr("text-anchor", "middle").attr("y", -4).attr("fill", "#fff").attr("font-size", function(d) { return Math.max(10, Math.min(16, d.r / 3)) + "px"; }).attr("font-weight", "600").attr("font-family", "Inter,system-ui,sans-serif").text(function(d) { return d.r > 25 ? d.data.name : ""; });
        node.append("text").attr("text-anchor", "middle").attr("y", 12).attr("fill", "rgba(255,255,255,0.7)").attr("font-size", "11px").attr("font-family", "Inter,system-ui,sans-serif").text(function(d) { return d.r > 20 ? d.data.count : ""; });
        node.append("title").text(function(d) { return d.data.name + ": " + d.data.count; });
      }
    } catch (e) { console.error("All entities chart error:", e); }
  }

  // --- Letter Page Bar Chart ---
  var ltDataEl = document.getElementById("letter-chart-data");
  var ltChartEl = document.getElementById("letter-chart");
  if (ltDataEl && ltChartEl && typeof d3 !== "undefined") {
    try {
      var ltData = JSON.parse(ltDataEl.textContent.trim());
      var ltEntries = (ltData.entries || []).slice(0, 15);
      var ltKey = ltData.taxonomyKey || "";
      if (ltEntries.length > 0) {
        var ltW = ltChartEl.clientWidth || 800;
        var ltBarH = 26;
        var ltGap = 4;
        var ltH = ltEntries.length * (ltBarH + ltGap);
        var ltLabelW = 180;
        var ltMax = d3.max(ltEntries, function(d) { return d.count; }) || 1;
        var ltScale = d3.scaleLinear().domain([0, ltMax]).range([0, ltW - ltLabelW - 80]);
        var svg = d3.select(ltChartEl).append("svg").attr("width", ltW).attr("height", ltH);
        ltEntries.forEach(function(d, i) {
          var y = i * (ltBarH + ltGap);
          var label = d.name.length > 22 ? d.name.substring(0, 20) + ".." : d.name;
          var g = svg.append("g").style("cursor", "pointer").on("click", function() { if (ltKey) window.location.href = "/" + ltKey + "/" + toSlug(d.name) + ".html"; });
          g.append("text").attr("x", ltLabelW - 8).attr("y", y + ltBarH / 2 + 4).attr("text-anchor", "end").attr("fill", "#9ca3af").attr("font-size", "13px").attr("font-family", "Inter,system-ui,sans-serif").text(label);
          g.append("rect").attr("x", ltLabelW).attr("y", y).attr("width", Math.max(ltScale(d.count), 4)).attr("height", ltBarH).attr("rx", 3).attr("fill", "#6366f1").attr("opacity", 0.85);
          g.append("text").attr("x", ltLabelW + Math.max(ltScale(d.count), 4) + 8).attr("y", y + ltBarH / 2 + 4).attr("fill", "#9ca3af").attr("font-size", "12px").attr("font-family", "Inter,system-ui,sans-serif").text(d.count);
        });
      }
    } catch (e) { console.error("Letter chart error:", e); }
  }

  // --- Mermaid Init ---
  if (typeof mermaid !== "undefined") {
    try {
      mermaid.initialize({
        startOnLoad: false,
        theme: "dark",
        themeVariables: {
          primaryColor: "#6366f1", primaryTextColor: "#e4e4e7",
          primaryBorderColor: "#818cf8", lineColor: "#2a2e3e",
          secondaryColor: "#1a1d27", tertiaryColor: "#22263a",
          background: "#1a1d27", mainBkg: "#1a1d27",
          nodeBorder: "#2a2e3e", clusterBkg: "#0f1117",
          clusterBorder: "#2a2e3e", titleColor: "#e4e4e7",
          edgeLabelBackground: "#1a1d27"
        }
      });
      mermaid.run();
    } catch (e) {
      console.error("Mermaid error:", e);
    }
  }

});

// --- Site Search ---
(function() {
  var overlay = document.getElementById("search-overlay");
  var input = document.getElementById("search-input");
  var resultsEl = document.getElementById("search-results");
  var toggleBtn = document.querySelector(".search-toggle");
  if (!overlay || !input || !resultsEl) return;

  var index = null;
  var activeIdx = -1;
  var results = [];

  function openSearch() {
    overlay.hidden = false;
    input.value = "";
    resultsEl.innerHTML = "";
    activeIdx = -1;
    input.focus();
    if (!index) loadIndex();
  }

  function closeSearch() {
    overlay.hidden = true;
    input.blur();
  }

  function loadIndex() {
    fetch("/search-index.json")
      .then(function(r) { return r.json(); })
      .then(function(data) { index = data; })
      .catch(function() { resultsEl.innerHTML = '<div class="search-no-results">Failed to load search index.</div>'; });
  }

  function search(query) {
    if (!index || !query) { resultsEl.innerHTML = ""; results = []; activeIdx = -1; return; }
    var q = query.toLowerCase();
    var tokens = q.split(/\s+/).filter(Boolean);

    var scored = [];
    for (var i = 0; i < index.length; i++) {
      var e = index[i];
      var haystack = (e.t + " " + (e.d || "") + " " + (e.n || "") + " " + (e.l || "") + " " + (e.m || "")).toLowerCase();
      var titleLower = e.t.toLowerCase();
      var allMatch = true;
      for (var j = 0; j < tokens.length; j++) {
        if (haystack.indexOf(tokens[j]) === -1) { allMatch = false; break; }
      }
      if (!allMatch) continue;

      var score = 0;
      if (titleLower === q) score += 100;
      else if (titleLower.indexOf(q) === 0) score += 50;
      else if (titleLower.indexOf(q) >= 0) score += 20;
      for (var k = 0; k < tokens.length; k++) {
        if (titleLower.indexOf(tokens[k]) >= 0) score += 5;
      }

      scored.push({ entry: e, score: score });
    }

    scored.sort(function(a, b) { return b.score - a.score; });
    results = scored.slice(0, 20);
    activeIdx = results.length > 0 ? 0 : -1;
    renderResults();
  }

  function renderResults() {
    if (results.length === 0) {
      resultsEl.innerHTML = input.value ? '<div class="search-no-results">No results found.</div>' : "";
      return;
    }
    var html = "";
    for (var i = 0; i < results.length; i++) {
      var e = results[i].entry;
      var cls = i === activeIdx ? "search-result active" : "search-result";
      html += '<a href="/' + e.s + '.html" class="' + cls + '">';
      html += '<div class="search-result-title">' + escHtml(e.t) + '</div>';
      if (e.d) html += '<div class="search-result-desc">' + escHtml(e.d) + '</div>';
      html += '<div class="search-result-meta">';
      if (e.n) html += '<span class="pill pill-accent">' + escHtml(e.n) + '</span>';
      if (e.l) html += '<span class="pill pill-blue">' + escHtml(e.l) + '</span>';
      if (e.m) html += '<span class="pill pill-green">' + escHtml(e.m) + '</span>';
      html += '</div></a>';
    }
    resultsEl.innerHTML = html;
  }

  function escHtml(s) {
    var d = document.createElement("div");
    d.appendChild(document.createTextNode(s));
    return d.innerHTML;
  }

  if (toggleBtn) toggleBtn.addEventListener("click", openSearch);

  overlay.addEventListener("click", function(e) {
    if (e.target === overlay) closeSearch();
  });

  input.addEventListener("input", function() { search(input.value.trim()); });

  input.addEventListener("keydown", function(e) {
    if (e.key === "Escape") { closeSearch(); }
    else if (e.key === "ArrowDown") { e.preventDefault(); if (activeIdx < results.length - 1) { activeIdx++; renderResults(); scrollActive(); } }
    else if (e.key === "ArrowUp") { e.preventDefault(); if (activeIdx > 0) { activeIdx--; renderResults(); scrollActive(); } }
    else if (e.key === "Enter" && activeIdx >= 0 && results[activeIdx]) { e.preventDefault(); window.location.href = "/" + results[activeIdx].entry.s + ".html"; }
  });

  function scrollActive() {
    var el = resultsEl.querySelector(".search-result.active");
    if (el) el.scrollIntoView({ block: "nearest" });
  }

  document.addEventListener("keydown", function(e) {
    if (overlay.hidden && e.key === "/" && !isInput(e.target)) {
      e.preventDefault();
      openSearch();
    }
    if (overlay.hidden && e.key === "k" && (e.metaKey || e.ctrlKey)) {
      e.preventDefault();
      openSearch();
    }
    if (!overlay.hidden && e.key === "Escape") {
      closeSearch();
    }
  });

  function isInput(el) {
    var tag = el.tagName;
    return tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT" || el.isContentEditable;
  }
})();
